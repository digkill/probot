package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/digkill/probot/internal/contacts"
	"github.com/digkill/probot/internal/domain"
	"github.com/digkill/probot/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	maxImportRows   = 5000
	maxImportBytes  = 25 << 20
	exportLinkTTL   = 5 * time.Minute
	exportTokenSize = 16 + 8 + sha256.Size
)

func (s *Server) contactRoutes(r chi.Router) {
	r.Get("/", s.handleListContacts)
	r.Post("/", s.handleCreateContact)
	r.Get("/stats", s.handleContactStats)
	r.Get("/tags", s.handleContactTags)
	r.Post("/import", s.handleImportContacts)
	r.Post("/export-link", s.handleContactExportLink)
	r.Patch("/{contactID}", s.handleUpdateContact)
	r.Delete("/{contactID}", s.handleDeleteContact)
}

func (s *Server) handleListContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	items, total, err := s.store.ListContacts(r.Context(), mustWorkspaceID(r), store.ContactFilter{
		Query: q.Get("q"), Tag: q.Get("tag"), Status: q.Get("status"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type contactInput struct {
	Email  string   `json:"email"`
	Phone  string   `json:"phone"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
}

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request) {
	var in contactInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	email, ok := contacts.NormalizeEmail(in.Email)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Enter a valid email.")
		return
	}
	if in.Status == "" {
		in.Status = contacts.StatusActive
	}
	if !contacts.ValidStatus(in.Status) {
		writeErr(w, http.StatusBadRequest, "Invalid value.")
		return
	}
	c, err := s.store.CreateContact(r.Context(), mustWorkspaceID(r), contacts.Row{
		Email: email, Phone: strings.TrimSpace(in.Phone), Name: strings.TrimSpace(in.Name),
		Tags: contacts.MergeTags(nil, cleanTags(in.Tags)), Status: in.Status,
	})
	if errors.Is(err, store.ErrContactExists) {
		writeErr(w, http.StatusConflict, store.ErrContactExists.Error())
		return
	}
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleUpdateContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid contact id")
		return
	}
	var p store.ContactPatch
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	if p.Status != nil && !contacts.ValidStatus(*p.Status) {
		writeErr(w, http.StatusBadRequest, "Invalid value.")
		return
	}
	if p.Tags != nil {
		t := contacts.MergeTags(nil, cleanTags(*p.Tags))
		p.Tags = &t
	}
	c, err := s.store.UpdateContact(r.Context(), mustWorkspaceID(r), id, p)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid contact id")
		return
	}
	if err := s.store.DeleteContact(r.Context(), mustWorkspaceID(r), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Not found.")
			return
		}
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContactStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.ContactStats(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleContactTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ContactTags(r.Context(), mustWorkspaceID(r))
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// handleImportContacts takes one batch of a CSV the browser parsed: the
// header plus up to maxImportRows records.
func (s *Server) handleImportContacts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	var req struct {
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
		Tags    []string   `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "Could not read the request.")
		return
	}
	if len(req.Rows) > maxImportRows {
		writeErr(w, http.StatusBadRequest, "Too many rows in one batch.")
		return
	}
	m, err := contacts.NewMapping(req.Columns)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	extra := cleanTags(req.Tags)
	batch := make([]contacts.Row, 0, len(req.Rows))
	invalid := 0
	for _, rec := range req.Rows {
		row, ok := m.Parse(rec)
		if !ok {
			invalid++
			continue
		}
		row.Tags = contacts.MergeTags(row.Tags, extra)
		batch = append(batch, row)
	}
	created, updated := 0, 0
	if len(batch) > 0 {
		created, updated, err = s.store.ImportContacts(r.Context(), mustWorkspaceID(r), m.Columns(), batch)
		if err != nil {
			writeCause(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "updated": updated, "invalid": invalid})
}

func (s *Server) handleContactExportLink(w http.ResponseWriter, r *http.Request) {
	token := s.exportToken(mustWorkspaceID(r), time.Now().Add(exportLinkTTL))
	writeJSON(w, http.StatusOK, map[string]string{"url": "/api/v1/contacts-export/" + token})
}

func (s *Server) handleContactExport(w http.ResponseWriter, r *http.Request) {
	wsID, ok := s.verifyExportToken(chi.URLParam(r, "token"), time.Now())
	if !ok {
		writeErr(w, http.StatusForbidden, "This download link has expired. Start the export again.")
		return
	}
	order, err := s.store.ContactFieldOrder(r.Context(), wsID)
	if err != nil {
		writeCause(w, http.StatusInternalServerError, err)
		return
	}
	cols := contacts.ExportColumns(order)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="contacts-`+time.Now().Format("20060102")+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	cw := csv.NewWriter(w)
	_ = cw.Write(cols)
	n := 0
	err = s.store.EachContact(r.Context(), wsID, func(c domain.Contact) error {
		if err := cw.Write(contacts.ExportRecord(cols, contacts.Row{
			Email: c.Email, Phone: c.Phone, Name: c.Name, Tags: c.Tags, Status: c.Status, Attributes: c.Attributes,
		})); err != nil {
			return err
		}
		if n++; n%2000 == 0 {
			cw.Flush()
		}
		return cw.Error()
	})
	cw.Flush()
	if err != nil {
		// Headers are already sent; the truncated file is the only signal left to the client.
		log.Printf("component=contacts operation=export status=failed err=%v", err)
	}
}

func (s *Server) exportKey() []byte {
	m := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	m.Write([]byte("contacts-export-v1"))
	return m.Sum(nil)
}

func (s *Server) exportToken(wsID uuid.UUID, exp time.Time) string {
	buf := make([]byte, 0, exportTokenSize)
	buf = append(buf, wsID[:]...)
	buf = binary.BigEndian.AppendUint64(buf, uint64(exp.Unix()))
	m := hmac.New(sha256.New, s.exportKey())
	m.Write(buf)
	return base64.RawURLEncoding.EncodeToString(m.Sum(buf))
}

func (s *Server) verifyExportToken(token string, now time.Time) (uuid.UUID, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != exportTokenSize {
		return uuid.Nil, false
	}
	payload, sig := raw[:24], raw[24:]
	m := hmac.New(sha256.New, s.exportKey())
	m.Write(payload)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return uuid.Nil, false
	}
	if now.Unix() > int64(binary.BigEndian.Uint64(payload[16:24])) {
		return uuid.Nil, false
	}
	wsID, _ := uuid.FromBytes(payload[:16])
	return wsID, true
}

func cleanTags(in []string) []string {
	var out []string
	for _, t := range in {
		out = append(out, contacts.SplitTags(t)...)
	}
	return out
}

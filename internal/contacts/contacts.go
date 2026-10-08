// Package contacts maps tabular contact exports (CSV columns) to contacts
// and back, keeping unknown columns as attributes so exports round-trip.
package contacts

import (
	"errors"
	"net/mail"
	"strings"
)

const (
	StatusActive       = "active"
	StatusUnsubscribed = "unsubscribed"
	StatusBounced      = "bounced"
	StatusComplained   = "complained"
)

func ValidStatus(s string) bool {
	switch s {
	case StatusActive, StatusUnsubscribed, StatusBounced, StatusComplained:
		return true
	}
	return false
}

type Row struct {
	Email      string
	Phone      string
	Name       string
	Tags       []string
	Status     string
	Attributes map[string]string
}

type core int

const (
	coreNone core = iota
	coreEmail
	corePhone
	coreName
	coreTags
	coreStatus
)

var aliases = map[string]core{
	"email": coreEmail, "e-mail": coreEmail, "почта": coreEmail, "email_address": coreEmail,
	"phone": corePhone, "телефон": corePhone, "phone_number": corePhone,
	"name": coreName, "имя": coreName, "first_name": coreName, "firstname": coreName,
	"tags": coreTags, "теги": coreTags, "tag": coreTags,
	"status": coreStatus,
}

func coreOf(column string) core { return aliases[normKey(column)] }

// IsCoreColumn reports whether a column is stored in a dedicated contact field.
func IsCoreColumn(column string) bool { return coreOf(column) != coreNone }

func normKey(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")))
}

type Mapping struct {
	columns []string
	kind    []core
	email   int
	lastIdx int
	reason  int
	intStat int
	conStat int
}

var ErrNoEmailColumn = errors.New("The file has no email column.")

func NewMapping(columns []string) (*Mapping, error) {
	m := &Mapping{email: -1, lastIdx: -1, reason: -1, intStat: -1, conStat: -1}
	for i, c := range columns {
		c = strings.TrimSpace(strings.TrimPrefix(c, "\ufeff"))
		m.columns = append(m.columns, c)
		k := coreOf(c)
		m.kind = append(m.kind, k)
		switch normKey(c) {
		case "last_name", "lastname", "фамилия":
			m.lastIdx = i
		case "email_unavailability_reason":
			m.reason = i
		case "integrated_email_status":
			m.intStat = i
		case "contact_status":
			m.conStat = i
		}
		if k == coreEmail && m.email < 0 {
			m.email = i
		}
	}
	if m.email < 0 {
		return nil, ErrNoEmailColumn
	}
	return m, nil
}

// Columns returns the header as it should be remembered for export.
func (m *Mapping) Columns() []string { return m.columns }

// Parse converts one record; ok is false when the email is missing or invalid.
func (m *Mapping) Parse(rec []string) (Row, bool) {
	get := func(i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	email, ok := NormalizeEmail(get(m.email))
	if !ok {
		return Row{}, false
	}
	row := Row{Email: email, Attributes: map[string]string{}}
	explicitStatus := ""
	for i, k := range m.kind {
		v := get(i)
		if v == "" {
			continue
		}
		switch k {
		case coreEmail:
		case corePhone:
			if row.Phone == "" {
				row.Phone = v
			}
		case coreName:
			if row.Name == "" {
				row.Name = v
			}
		case coreTags:
			row.Tags = MergeTags(row.Tags, SplitTags(v))
		case coreStatus:
			explicitStatus = strings.ToLower(v)
		default:
			row.Attributes[m.columns[i]] = v
		}
	}
	if row.Name == "" {
		row.Name = get(m.lastIdx)
	}
	if ValidStatus(explicitStatus) {
		row.Status = explicitStatus
	} else {
		row.Status = deriveStatus(get(m.intStat), get(m.reason), get(m.conStat))
	}
	return row, true
}

func deriveStatus(integrated, reason, contactStatus string) string {
	switch strings.ToLower(reason) {
	case "unsubscribed":
		return StatusUnsubscribed
	case "spam_folder", "spam_rejected", "spam", "complained", "complaint":
		return StatusComplained
	case "blocked", "unreachable", "mailbox_full", "temp_unreachable", "bounced", "invalid", "hard_bounce":
		return StatusBounced
	}
	if strings.EqualFold(integrated, "unavailable") {
		return StatusBounced
	}
	if strings.EqualFold(contactStatus, "disabled") {
		return StatusUnsubscribed
	}
	return StatusActive
}

func NormalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \t\r\n<>,;\"") {
		return "", false
	}
	at := strings.LastIndex(s, "@")
	if at < 1 || at == len(s)-1 || !strings.Contains(s[at+1:], ".") {
		return "", false
	}
	if _, err := mail.ParseAddress(s); err != nil {
		return "", false
	}
	return s, true
}

func SplitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" && len(t) <= 100 {
			out = append(out, t)
		}
	}
	return out
}

func MergeTags(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, t := range append(append([]string{}, a...), b...) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// ExportColumns returns the CSV header: the remembered source layout, or a
// default one, always ending with our status column.
func ExportColumns(remembered []string) []string {
	cols := remembered
	if len(cols) == 0 {
		cols = []string{"email", "phone", "name", "tags"}
	}
	out := append([]string{}, cols...)
	for _, c := range cols {
		if coreOf(c) == coreStatus {
			return out
		}
	}
	return append(out, "status")
}

// ExportRecord renders one contact in the order of columns.
func ExportRecord(columns []string, r Row) []string {
	rec := make([]string, len(columns))
	for i, c := range columns {
		switch coreOf(c) {
		case coreEmail:
			rec[i] = r.Email
		case corePhone:
			rec[i] = r.Phone
		case coreName:
			rec[i] = r.Name
		case coreTags:
			rec[i] = strings.Join(r.Tags, ",")
		case coreStatus:
			rec[i] = r.Status
		default:
			rec[i] = r.Attributes[c]
		}
	}
	return rec
}

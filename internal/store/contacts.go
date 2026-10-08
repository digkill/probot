package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/digkill/probot/internal/contacts"
	"github.com/digkill/probot/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrContactExists = errors.New("A contact with this email already exists.")

const contactCols = `id, workspace_id, email, phone, name, tags, status, attributes, created_at, updated_at`

type contactScanner interface{ Scan(dest ...any) error }

func scanContact(row contactScanner) (domain.Contact, error) {
	var c domain.Contact
	var attrs []byte
	if err := row.Scan(&c.ID, &c.WorkspaceID, &c.Email, &c.Phone, &c.Name, &c.Tags, &c.Status, &attrs, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	c.Attributes = map[string]string{}
	if len(attrs) > 0 {
		if err := json.Unmarshal(attrs, &c.Attributes); err != nil {
			return c, err
		}
	}
	if c.Tags == nil {
		c.Tags = []string{}
	}
	return c, nil
}

type ContactFilter struct {
	Query  string
	Tag    string
	Status string
	Limit  int
	Offset int
}

func (f ContactFilter) where(workspaceID uuid.UUID) (string, []any) {
	q := "workspace_id=$1"
	args := []any{workspaceID}
	add := func(cond string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND "+cond, len(args))
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		add("(email || ' ' || name || ' ' || phone) ILIKE $%d", "%"+escapeLike(s)+"%")
	}
	if f.Tag != "" {
		add("tags @> ARRAY[$%d]::text[]", f.Tag)
	}
	if f.Status != "" {
		add("status=$%d", f.Status)
	}
	return q, args
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Store) ListContacts(ctx context.Context, workspaceID uuid.UUID, f ContactFilter) ([]domain.Contact, int, error) {
	where, args := f.where(workspaceID)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM contacts WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	args = append(args, f.Limit, max(f.Offset, 0))
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM contacts WHERE %s ORDER BY created_at DESC, id LIMIT $%d OFFSET $%d`,
		contactCols, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (s *Store) CreateContact(ctx context.Context, workspaceID uuid.UUID, r contacts.Row) (domain.Contact, error) {
	attrs, _ := json.Marshal(nonNilAttrs(r.Attributes))
	c, err := scanContact(s.Pool.QueryRow(ctx, `
		INSERT INTO contacts (workspace_id, email, phone, name, tags, status, attributes)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+contactCols, workspaceID, r.Email, r.Phone, r.Name, nonNilTags(r.Tags), r.Status, attrs))
	if isUniqueViolation(err, "") {
		return c, ErrContactExists
	}
	return c, err
}

type ContactPatch struct {
	Phone  *string   `json:"phone"`
	Name   *string   `json:"name"`
	Tags   *[]string `json:"tags"`
	Status *string   `json:"status"`
}

func (s *Store) UpdateContact(ctx context.Context, workspaceID, id uuid.UUID, p ContactPatch) (domain.Contact, error) {
	var tags any
	if p.Tags != nil {
		tags = nonNilTags(*p.Tags)
	}
	return scanContact(s.Pool.QueryRow(ctx, `
		UPDATE contacts SET
			phone = COALESCE($3, phone),
			name = COALESCE($4, name),
			tags = COALESCE($5::text[], tags),
			status = COALESCE($6, status),
			updated_at = now()
		WHERE id=$1 AND workspace_id=$2
		RETURNING `+contactCols, id, workspaceID, p.Phone, p.Name, tags, p.Status))
}

func (s *Store) DeleteContact(ctx context.Context, workspaceID, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM contacts WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ContactStats(ctx context.Context, workspaceID uuid.UUID) (map[string]int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT status, COUNT(*) FROM contacts WHERE workspace_id=$1 GROUP BY status`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{"total": 0, contacts.StatusActive: 0, contacts.StatusUnsubscribed: 0, contacts.StatusBounced: 0, contacts.StatusComplained: 0}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
		out["total"] += n
	}
	return out, rows.Err()
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

func (s *Store) ContactTags(ctx context.Context, workspaceID uuid.UUID) ([]TagCount, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t, COUNT(*) FROM contacts, unnest(tags) AS t
		WHERE workspace_id=$1 GROUP BY t ORDER BY COUNT(*) DESC, t LIMIT 200
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

const tagSep = "\x1f"

// ImportContacts upserts a batch. Existing contacts keep a non-active status
// so a re-import never resubscribes people who unsubscribed or bounced.
func (s *Store) ImportContacts(ctx context.Context, workspaceID uuid.UUID, columns []string, batch []contacts.Row) (created, updated int, err error) {
	byEmail := make(map[string]int, len(batch))
	var rows []contacts.Row
	for _, r := range batch {
		if i, ok := byEmail[r.Email]; ok {
			prev := &rows[i]
			prev.Tags = contacts.MergeTags(prev.Tags, r.Tags)
			for k, v := range r.Attributes {
				prev.Attributes[k] = v
			}
			if r.Phone != "" {
				prev.Phone = r.Phone
			}
			if r.Name != "" {
				prev.Name = r.Name
			}
			if prev.Status == contacts.StatusActive {
				prev.Status = r.Status
			}
			continue
		}
		byEmail[r.Email] = len(rows)
		rows = append(rows, r)
	}
	emails := make([]string, len(rows))
	phones := make([]string, len(rows))
	names := make([]string, len(rows))
	tags := make([]string, len(rows))
	statuses := make([]string, len(rows))
	attrs := make([]string, len(rows))
	for i, r := range rows {
		emails[i], phones[i], names[i], statuses[i] = r.Email, r.Phone, r.Name, r.Status
		tags[i] = strings.Join(r.Tags, tagSep)
		raw, _ := json.Marshal(nonNilAttrs(r.Attributes))
		attrs[i] = string(raw)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	if len(columns) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO contact_fields (workspace_id, key, position)
			SELECT $1, k, (SELECT COALESCE(MAX(position), -1) FROM contact_fields WHERE workspace_id=$1) + ord
			FROM unnest($2::text[]) WITH ORDINALITY AS u(k, ord)
			WHERE k <> ''
			ON CONFLICT DO NOTHING
		`, workspaceID, columns); err != nil {
			return 0, 0, err
		}
	}
	res, err := tx.Query(ctx, `
		INSERT INTO contacts (workspace_id, email, phone, name, tags, status, attributes)
		SELECT $1, e, p, n, CASE WHEN t = '' THEN '{}'::text[] ELSE string_to_array(t, E'\x1f') END, s, a::jsonb
		FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[]) AS u(e, p, n, t, s, a)
		ON CONFLICT (workspace_id, email) DO UPDATE SET
			phone = CASE WHEN EXCLUDED.phone <> '' THEN EXCLUDED.phone ELSE contacts.phone END,
			name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE contacts.name END,
			tags = ARRAY(SELECT DISTINCT x FROM unnest(contacts.tags || EXCLUDED.tags) AS x),
			status = CASE WHEN contacts.status <> 'active' THEN contacts.status ELSE EXCLUDED.status END,
			attributes = contacts.attributes || EXCLUDED.attributes,
			updated_at = now()
		RETURNING (xmax = 0)
	`, workspaceID, emails, phones, names, tags, statuses, attrs)
	if err != nil {
		return 0, 0, err
	}
	for res.Next() {
		var inserted bool
		if err := res.Scan(&inserted); err != nil {
			res.Close()
			return 0, 0, err
		}
		if inserted {
			created++
		} else {
			updated++
		}
	}
	res.Close()
	if err := res.Err(); err != nil {
		return 0, 0, err
	}
	return created, updated, tx.Commit(ctx)
}

func (s *Store) ContactFieldOrder(ctx context.Context, workspaceID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key FROM contact_fields WHERE workspace_id=$1 ORDER BY position, key`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// EachContact streams all contacts of a workspace in creation order.
func (s *Store) EachContact(ctx context.Context, workspaceID uuid.UUID, fn func(domain.Contact) error) error {
	rows, err := s.Pool.Query(ctx, `SELECT `+contactCols+` FROM contacts WHERE workspace_id=$1 ORDER BY created_at, id`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return err
		}
		if err := fn(c); err != nil {
			return err
		}
	}
	return rows.Err()
}

func nonNilTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

func nonNilAttrs(a map[string]string) map[string]string {
	if a == nil {
		return map[string]string{}
	}
	return a
}

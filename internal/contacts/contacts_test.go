package contacts

import (
	"reflect"
	"testing"
)

var header = []string{"id", "email", "integrated_email_status", "email_unavailability_reason", "phone", "tags", "contact_status", "Name", "last_name", "utm_source"}

func TestParseMapsCoreFieldsAndKeepsTheRest(t *testing.T) {
	m, err := NewMapping(header)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := m.Parse([]string{"17", " Ann@Example.COM ", "active", "", "79991234567", "tilda,покупки", "active", "Анна", "", "yandex"})
	if !ok {
		t.Fatal("valid row rejected")
	}
	if r.Email != "ann@example.com" || r.Phone != "79991234567" || r.Name != "Анна" || r.Status != StatusActive {
		t.Fatalf("%+v", r)
	}
	if !reflect.DeepEqual(r.Tags, []string{"tilda", "покупки"}) {
		t.Fatalf("tags %v", r.Tags)
	}
	want := map[string]string{"id": "17", "integrated_email_status": "active", "contact_status": "active", "utm_source": "yandex"}
	if !reflect.DeepEqual(r.Attributes, want) {
		t.Fatalf("attributes %v", r.Attributes)
	}
}

func TestStatusIsDerivedFromSourceColumns(t *testing.T) {
	m, _ := NewMapping(header)
	for reason, want := range map[string]string{
		"unsubscribed": StatusUnsubscribed, "spam_folder": StatusComplained, "spam_rejected": StatusComplained,
		"blocked": StatusBounced, "mailbox_full": StatusBounced, "temp_unreachable": StatusBounced, "unreachable": StatusBounced,
	} {
		r, _ := m.Parse([]string{"1", "a@b.co", "unavailable", reason, "", "", "active", "", "", ""})
		if r.Status != want {
			t.Errorf("%s -> %s, want %s", reason, r.Status, want)
		}
	}
	if r, _ := m.Parse([]string{"1", "a@b.co", "active", "", "", "", "disabled", "", "", ""}); r.Status != StatusUnsubscribed {
		t.Errorf("disabled contact -> %s", r.Status)
	}
	if r, _ := m.Parse([]string{"1", "a@b.co", "new", "", "", "", "active", "", "Петров", ""}); r.Status != StatusActive || r.Name != "Петров" {
		t.Errorf("new contact -> %+v", r)
	}
}

func TestInvalidEmailsAndMissingColumn(t *testing.T) {
	if _, err := NewMapping([]string{"phone", "name"}); err != ErrNoEmailColumn {
		t.Fatalf("err=%v", err)
	}
	m, _ := NewMapping(header)
	for _, e := range []string{"", "no-at", "a@b", "a b@c.ru", "x@y.ru, z@w.ru", "@d.ru"} {
		if _, ok := m.Parse([]string{"1", e, "", "", "", "", "", "", "", ""}); ok {
			t.Errorf("accepted %q", e)
		}
	}
}

func TestExportRoundTrip(t *testing.T) {
	m, _ := NewMapping(header)
	src := []string{"17", "ann@example.com", "unavailable", "unsubscribed", "79991234567", "tilda,покупки", "active", "Анна", "", "yandex"}
	r, _ := m.Parse(src)
	cols := ExportColumns(m.Columns())
	if cols[len(cols)-1] != "status" || len(cols) != len(header)+1 {
		t.Fatalf("cols %v", cols)
	}
	got := ExportRecord(cols, r)
	if !reflect.DeepEqual(got[:len(src)], src) || got[len(src)] != StatusUnsubscribed {
		t.Fatalf("got %v", got)
	}
	again, _ := NewMapping(cols)
	r2, _ := again.Parse(got)
	if r2.Status != StatusUnsubscribed || !reflect.DeepEqual(r2.Attributes, r.Attributes) {
		t.Fatalf("re-import %+v", r2)
	}
}

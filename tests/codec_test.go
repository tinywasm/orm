package tests

import (
	"testing"
	"github.com/tinywasm/fmt"
)

type mockFieldWriter struct {
	data map[string]any
}

func (m *mockFieldWriter) String(name, val string)  { m.data[name] = val }
func (m *mockFieldWriter) Int(name string, val int64) { m.data[name] = val }
func (m *mockFieldWriter) Uint(name string, val uint64) { m.data[name] = val }
func (m *mockFieldWriter) Float(name string, val float64) { m.data[name] = val }
func (m *mockFieldWriter) Bool(name string, val bool) { m.data[name] = val }
func (m *mockFieldWriter) Bytes(name string, val []byte) { m.data[name] = val }
func (m *mockFieldWriter) Null(name string) { m.data[name] = nil }
func (m *mockFieldWriter) Object(name string, val fmt.Encodable) {
	inner := &mockFieldWriter{data: make(map[string]any)}
	val.EncodeFields(inner)
	m.data[name] = inner.data
}
func (m *mockFieldWriter) Array(name string, n int, each func(i int, a fmt.ArrayWriter)) {
}

type mockFieldReader struct {
	data map[string]any
}

func (m *mockFieldReader) String(name string) (string, bool) {
	v, ok := m.data[name].(string)
	return v, ok
}
func (m *mockFieldReader) Int(name string) (int64, bool) {
	v, ok := m.data[name].(int64)
	return v, ok
}
func (m *mockFieldReader) Uint(name string) (uint64, bool) {
	v, ok := m.data[name].(uint64)
	return v, ok
}
func (m *mockFieldReader) Float(name string) (float64, bool) {
	v, ok := m.data[name].(float64)
	return v, ok
}
func (m *mockFieldReader) Bool(name string) (bool, bool) {
	v, ok := m.data[name].(bool)
	return v, ok
}
func (m *mockFieldReader) Bytes(name string) ([]byte, bool) {
	v, ok := m.data[name].([]byte)
	return v, ok
}
func (m *mockFieldReader) Object(name string, into fmt.Decodable) bool {
	v, ok := m.data[name].(map[string]any)
	if !ok { return false }
	inner := &mockFieldReader{data: v}
	into.DecodeFields(inner)
	return true
}
func (m *mockFieldReader) Array(name string) (fmt.ArrayReader, bool) {
	return nil, false
}

func TestCodec(t *testing.T) {
	u := &User{
		ID:        1,
		FirstName: "John",
		LastName:  "Doe",
		Email:     "john@example.com",
		Score:     42.5,
		IsActive:  true,
		Avatar:    []byte("avatar_data"),
	}

	w := &mockFieldWriter{data: make(map[string]any)}
	u.EncodeFields(w)

	if v, _ := w.data["id"].(int64); v != 1 { t.Errorf("expected 1, got %v", w.data["id"]) }
	if w.data["first_name"] != "John" { t.Errorf("expected John, got %v", w.data["first_name"]) }
	if w.data["score"] != 42.5 { t.Errorf("expected 42.5, got %v", w.data["score"]) }
	if w.data["is_active"] != true { t.Errorf("expected true, got %v", w.data["is_active"]) }

	u2 := &User{}
	r := &mockFieldReader{data: w.data}
	u2.DecodeFields(r)

	if u2.ID != 1 { t.Errorf("expected 1, got %v", u2.ID) }
	if u2.FirstName != "John" { t.Errorf("expected John, got %v", u2.FirstName) }
	if u2.Score != 42.5 { t.Errorf("expected 42.5, got %v", u2.Score) }
	if u2.IsActive != true { t.Errorf("expected true, got %v", u2.IsActive) }

	// Test nested object
	uwj := &UserWithJSON{
		ID:   "user1",
		Name: "Jane",
		HomeAddr: Address{
			Street: "Main St",
			City:   "Springfield",
		},
	}

	w2 := &mockFieldWriter{data: make(map[string]any)}
	uwj.EncodeFields(w2)

	addrData, ok := w2.data["home_addr"].(map[string]any)
	if !ok { t.Fatalf("home_addr not found or not a map") }
	if addrData["street"] != "Main St" { t.Errorf("expected Main St, got %v", addrData["street"]) }

	uwj2 := &UserWithJSON{}
	r2 := &mockFieldReader{data: w2.data}
	uwj2.DecodeFields(r2)

	if uwj2.HomeAddr.Street != "Main St" { t.Errorf("expected Main St, got %v", uwj2.HomeAddr.Street) }
}

func TestPointerField(t *testing.T) {
	wp := &WithPointers{
		ID: "wp1",
		Addr: &Address{
			Street: "Ptr St",
			City:   "Ptr City",
		},
	}

	w := &mockFieldWriter{data: make(map[string]any)}
	wp.EncodeFields(w)

	addrData, ok := w.data["addr"].(map[string]any)
	if !ok { t.Fatalf("addr not found or not a map") }
	if addrData["street"] != "Ptr St" { t.Errorf("expected Ptr St, got %v", addrData["street"]) }

	// Test nil pointer
	wpNil := &WithPointers{ID: "wp2", Addr: nil}
	w2 := &mockFieldWriter{data: make(map[string]any)}
	wpNil.EncodeFields(w2)
	if w2.data["addr"] != nil { t.Errorf("expected nil for addr, got %v", w2.data["addr"]) }

	wp2 := &WithPointers{}
	r := &mockFieldReader{data: w.data}
	wp2.DecodeFields(r)
	if wp2.Addr == nil { t.Fatalf("expected Addr to be populated, got nil") }
	if wp2.Addr.Street != "Ptr St" { t.Errorf("expected Ptr St, got %v", wp2.Addr.Street) }

	wp3 := &WithPointers{}
	r2 := &mockFieldReader{data: w2.data}
	wp3.DecodeFields(r2)
	if wp3.Addr != nil { t.Errorf("expected Addr to be nil, got %v", wp3.Addr) }
}

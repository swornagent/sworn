package protocol

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"testing"
)

func testSnapshotBytes(t *testing.T, name string) []byte {
	t.Helper()
	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, name)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func testJSONObject(t *testing.T, contents []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		t.Fatal(err)
	}
	return object
}

func testJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	contents, err := EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func testNestedObject(t *testing.T, object map[string]any, name string) map[string]any {
	t.Helper()
	nested, ok := object[name].(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want object", name, object[name])
	}
	return nested
}

func testObjectArray(t *testing.T, object map[string]any, name string) []any {
	t.Helper()
	array, ok := object[name].([]any)
	if !ok {
		t.Fatalf("%s = %T, want array", name, object[name])
	}
	return array
}

func testArrayObject(t *testing.T, array []any, index int) map[string]any {
	t.Helper()
	object, ok := array[index].(map[string]any)
	if !ok {
		t.Fatalf("array[%d] = %T, want object", index, array[index])
	}
	return object
}

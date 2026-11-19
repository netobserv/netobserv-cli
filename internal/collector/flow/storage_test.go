package flow

import (
	"path/filepath"
	"testing"
)

func TestOpenFlowDB(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Test success case
	// {"Bytes":32}
	bs := []byte{123, 34, 66, 121, 116, 101, 115, 34, 58, 51, 50, 125}
	err = Insert(db, bs)
	if err != nil {
		t.Errorf("Unexpected error inserting flow: %v", err)
	}

	q, err := Query(db, "SELECT Bytes FROM flow")
	if err != nil {
		t.Error("Unexpected error querying flow")
	}
	if len(q) != 1 {
		t.Error("Expected 1 row in flow table")
	}

	if q[0] != "32" {
		t.Error("Unexpected result from query")
	}

	// Test DB error case
	db.Close()
	err = Insert(db, []byte("1"))
	if err == nil {
		t.Error("Expected error for closed DB")
	}
}

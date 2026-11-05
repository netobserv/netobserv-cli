// Package flow stores captured flow records in SQLite.
package flow

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/netobserv/flowlogs-pipeline/pkg/config"
	// need to import the sqlite3 driver
	_ "github.com/mattn/go-sqlite3"
)

// Open opens a flow database and ensures its schema exists.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open flow database: %w", err)
	}
	if err := CreateTable(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Insert stores one flow JSON record in the database.
func Insert(db *sql.DB, record []byte) error {
	return insertFlow(db, record)
}

// CreateTable creates the flow table if it does not already exist.
func CreateTable(db *sql.DB) error {
	createFlowsTableSQL := `CREATE TABLE flow (
		"DnsErrno" INTEGER,
		"Dscp" INTEGER,
		"DstAddr" TEXT ,
		"DstPort" INTEGER,
		"Interface" TEXT,
		"Proto" INTEGER,
		"SrcAddr" TEXT,
		"SrcPort" INTEGER,
		"Bytes" INTEGER,
		"Packets" INTEGER,
		"PktDropLatestDropCause" TEXT,
		"PktDropBytes" INTEGER,
		"PktDropPackets" INTEGER,
		"DnsId" INTEGER,
		"DnsName" TEXT,
		"DnsFlagsResponseCode" TEXT,
		"DnsLatencyMs" TIMESTAMP,
		"TimeFlowRTTNs" TIMESTAMP
	  );` // SQL Statement for Create Table

	statement, err := db.Prepare(createFlowsTableSQL) // Prepare SQL Statement
	if err != nil {
		return err
	}
	_, err = statement.Exec() // Execute SQL Statements
	if err != nil {
		return err
	}
	return nil
}

func insertFlow(db *sql.DB, buf []byte) error {
	flow := config.GenericMap{}

	// Unmarshal the JSON string into the flow object
	err := json.Unmarshal(buf, &flow)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}
	// Insert message into database
	var flowSQL string
	switch {
	case flow["PktDropPackets"] != 0 && flow["DnsId"] != 0:
		flowSQL =
			`INSERT INTO flow(DnsErrno, Dscp, DstAddr, DstPort, Interface, Proto, SrcAddr, SrcPort, Bytes, Packets, PktDropLatestDropCause, PktDropBytes, PktDropPackets, DnsId, DnsName, DnsFlagsResponseCode, DnsLatencyMs, TimeFlowRttNs) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	case flow["PktDropPackets"] != 0:
		flowSQL =
			`INSERT INTO flow(DnsErrno, Dscp, DstAddr, DstPort, Interface, Proto, SrcAddr, SrcPort, Bytes, Packets, PktDropLatestDropCause, PktDropBytes, PktDropPackets, TimeFlowRttNs) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	case flow["DnsId"] != 0:
		flowSQL =
			`INSERT INTO flow(DnsErrno, Dscp, DstAddr, DstPort, Interface, Proto, SrcAddr, SrcPort, Bytes, Packets, DnsId, DnsName, DnsFlagsResponseCode, DnsLatencyMs, TimeFlowRttNs) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	default:
		flowSQL =
			`INSERT INTO flow(DnsErrno, Dscp, DstAddr, DstPort, Interface, Proto, SrcAddr, SrcPort, Bytes, Packets, TimeFlowRttNs) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	statement, err := db.Prepare(flowSQL) // Prepare statement.
	// This is good to avoid SQL injections
	if err != nil {
		return fmt.Errorf("error preparing SQL: %v", err.Error())
	}

	switch {
	case flow["PktDropLatestDropCause"] != 0 && flow["DnsId"] != 0:
		_, err = statement.Exec(
			flow["DNSErrno"], flow["Dscp"], flow["DstAddr"], flow["DstPort"], flow["Interface"],
			flow["Proto"], flow["SrcAddr"], flow["SrcPort"], flow["Bytes"], flow["Packets"],
			flow["PktDropLatestDropCause"], flow["PktDropBytes"], flow["PktDropPackets"],
			flow["DnsId"], flow["DnsName"], flow["DnsFlagsResponseCode"], flow["DnsLatencyMs"],
			flow["TimeFlowRttNs"])
	case flow["PktDropLatestDropCause"] != 0:
		_, err = statement.Exec(
			flow["DNSErrno"], flow["Dscp"], flow["DstAddr"], flow["DstPort"], flow["Interface"],
			flow["Proto"], flow["SrcAddr"], flow["SrcPort"], flow["Bytes"], flow["Packets"],
			flow["PktDropLatestDropCause"], flow["PktDropBytes"], flow["PktDropPackets"],
			flow["TimeFlowRttNs"])
	case flow["DnsId"] != 0:
		_, err = statement.Exec(
			flow["DNSErrno"], flow["Dscp"], flow["DstAddr"], flow["DstPort"], flow["Interface"],
			flow["Proto"], flow["SrcAddr"], flow["SrcPort"], flow["Bytes"], flow["Packets"],
			flow["DnsId"], flow["DnsName"], flow["DnsFlagsResponseCode"], flow["DnsLatencyMs"],
			flow["TimeFlowRttNs"])
	default:
		_, err = statement.Exec(
			flow["DNSErrno"], flow["Dscp"], flow["DstAddr"], flow["DstPort"], flow["Interface"],
			flow["Proto"], flow["SrcAddr"], flow["SrcPort"], flow["Bytes"], flow["Packets"],
			flow["TimeFlowRttNs"])
	}
	if err != nil {
		return fmt.Errorf("error inserting into database: %v", err.Error())
	}
	return nil
}

// Query executes a query and returns the first column of each row as text.
func Query(db *sql.DB, query string) ([]string, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %v", err.Error())
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err.Error())
		}
		result = append(result, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %v", err.Error())
	}

	return result, nil
}

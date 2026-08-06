package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestReleaseTypeRoundTrips(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "release_types", `
		id bigint PRIMARY KEY,
		int2_value smallint NOT NULL,
		int4_value integer NOT NULL,
		float4_value real NOT NULL,
		float8_value double precision NOT NULL,
		numeric_value numeric(40,20) NOT NULL,
		date_value date NOT NULL,
		timestamp_value timestamp NOT NULL,
		uuid_value uuid NOT NULL,
		json_value json NOT NULL,
		jsonb_value jsonb NOT NULL,
		nullable_value text
	`)
	created := time.Date(2026, 8, 6, 12, 34, 56, 123456000, time.UTC)
	const numericValue = "12345678901234567890.12345678901234567890"
	const uuidValue = "0198f96d-8de1-7a52-a270-6f6973746772"
	const jsonValue = `{"identity":true,"count":2}`
	_, err := db.ExecContext(context.Background(),
		"INSERT INTO "+table+"(id,int2_value,int4_value,float4_value,float8_value,numeric_value,date_value,timestamp_value,uuid_value,json_value,jsonb_value,nullable_value) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
		int64(1), int16(12), int32(34), float32(1.25), float64(2.5), numericValue, created, created, uuidValue, jsonValue, jsonValue, nil)
	if err != nil {
		t.Fatal(err)
	}
	var (
		id, int2Value, int4Value    int64
		float4Value, float8Value    float64
		numericResult, uuidResult   string
		dateResult, timestampResult time.Time
		jsonResult, jsonbResult     []byte
		nullable                    sql.NullString
	)
	if err := db.QueryRowContext(context.Background(),
		"SELECT id,int2_value,int4_value,float4_value,float8_value,numeric_value,date_value,timestamp_value,uuid_value,json_value,jsonb_value,nullable_value FROM "+table+" WHERE id=$1", int64(1)).
		Scan(&id, &int2Value, &int4Value, &float4Value, &float8Value, &numericResult, &dateResult, &timestampResult, &uuidResult, &jsonResult, &jsonbResult, &nullable); err != nil {
		t.Fatal(err)
	}
	if id != 1 || int2Value != 12 || int4Value != 34 || math.Abs(float4Value-1.25) > 0.00001 || math.Abs(float8Value-2.5) > 0.0000001 {
		t.Fatalf("unexpected scalar values")
	}
	if numericResult != numericValue || uuidResult != uuidValue || nullable.Valid {
		t.Fatalf("unexpected numeric/uuid/null values: %q %q %#v", numericResult, uuidResult, nullable)
	}
	if dateResult.Format("2006-01-02") != created.Format("2006-01-02") || !timestampResult.Equal(created) {
		t.Fatalf("unexpected time values: %v %v", dateResult, timestampResult)
	}
	for _, raw := range [][]byte{jsonResult, jsonbResult} {
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil || decoded["identity"] != true {
			t.Fatalf("unexpected JSON %s: %v", raw, err)
		}
	}
}

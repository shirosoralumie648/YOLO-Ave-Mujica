package jobs

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type fakeJobRow struct {
	values []any
}

func defaultFakeJobRowValues() []any {
	createdAt := time.Unix(1700000000, 0).UTC()
	return []any{
		int64(1),
		int64(1),
		int64(42),
		int64(9),
		"zero-shot",
		StatusQueued,
		"gpu",
		[]byte(`[]`),
		"idem-nullables",
		nil,
		[]byte(`{"prompt":"person"}`),
		[]byte(`[]`),
		[]byte(`{}`),
		0,
		0,
		0,
		createdAt,
		nil,
		nil,
		nil,
		0,
		nil,
		nil,
	}
}

func (r fakeJobRow) Scan(dest ...any) error {
	values := defaultFakeJobRowValues()
	if len(r.values) > 0 {
		values = r.values
	}

	for i, value := range values {
		switch ptr := dest[i].(type) {
		case *int64:
			if value == nil {
				return fmt.Errorf("can't scan into dest[%d]: cannot scan NULL into *int64", i)
			}
			*ptr = value.(int64)
		case *sql.NullInt64:
			if value == nil {
				*ptr = sql.NullInt64{}
				continue
			}
			*ptr = sql.NullInt64{Int64: value.(int64), Valid: true}
		case *string:
			if value == nil {
				return fmt.Errorf("can't scan into dest[%d]: cannot scan NULL into *string", i)
			}
			*ptr = value.(string)
		case **string:
			if value == nil {
				*ptr = nil
				continue
			}
			v := value.(string)
			*ptr = &v
		case *[]byte:
			*ptr = value.([]byte)
		case *int:
			*ptr = value.(int)
		case *time.Time:
			*ptr = value.(time.Time)
		case **time.Time:
			if value == nil {
				*ptr = nil
				continue
			}
			v := value.(time.Time)
			*ptr = &v
		default:
			return fmt.Errorf("unexpected dest type at %d: %T", i, dest[i])
		}
	}

	return nil
}

func TestScanJobHandlesNullOptionalFields(t *testing.T) {
	job, err := scanJob(fakeJobRow{})
	if err != nil {
		t.Fatalf("scanJob returned error: %v", err)
	}
	if job.WorkerID != "" {
		t.Fatalf("expected empty worker id, got %q", job.WorkerID)
	}
	if job.ErrorCode != "" || job.ErrorMsg != "" {
		t.Fatalf("expected empty error fields, got code=%q msg=%q", job.ErrorCode, job.ErrorMsg)
	}
}

func TestScanJobHandlesNullDatasetAndSnapshotIDs(t *testing.T) {
	values := defaultFakeJobRowValues()
	values[2] = nil
	values[3] = nil

	_, err := scanJob(fakeJobRow{values: values})
	if err != nil {
		t.Fatalf("scanJob returned error: %v", err)
	}
}

func TestScanJobHydratesPersistedResultRef(t *testing.T) {
	values := defaultFakeJobRowValues()
	values[11] = []byte(`[9,12]`)
	values[12] = []byte(`{"result_type":"artifacts","result_count":2,"artifact_ids":[9,12]}`)

	job, err := scanJob(fakeJobRow{values: values})
	if err != nil {
		t.Fatalf("scanJob returned error: %v", err)
	}
	if job.ResultType != "artifacts" {
		t.Fatalf("expected result_type artifacts, got %q", job.ResultType)
	}
	if job.ResultCount != 2 {
		t.Fatalf("expected result_count 2, got %d", job.ResultCount)
	}
	if !reflect.DeepEqual(job.ResultArtifactIDs, []int64{9, 12}) {
		t.Fatalf("expected artifact ids [9 12], got %+v", job.ResultArtifactIDs)
	}
}

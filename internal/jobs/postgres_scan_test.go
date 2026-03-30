package jobs

import (
	"fmt"
	"testing"
	"time"
)

type fakeJobRow struct{}

func (fakeJobRow) Scan(dest ...any) error {
	createdAt := time.Unix(1700000000, 0).UTC()
	values := []any{
		int64(1),
		int64(1),
		"zero-shot",
		StatusQueued,
		"gpu",
		"idem-nullables",
		nil,
		[]byte(`{"prompt":"person"}`),
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

	for i, value := range values {
		switch ptr := dest[i].(type) {
		case *int64:
			*ptr = value.(int64)
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

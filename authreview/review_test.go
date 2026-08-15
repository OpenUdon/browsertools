package authreview

import (
	"os"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authprofile"
)

func TestBuildVerifyAndExpiry(t *testing.T) {
	data, err := os.ReadFile("../authprofile/testdata/valid-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	value, err := authprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	bundle, err := Build(value, current)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable || len(bundle.Gaps) != 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
	if err := Verify(bundle, current); err != nil {
		t.Fatal(err)
	}
	expired, err := Build(value, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if expired.Promotable || len(expired.Gaps) != 1 {
		t.Fatalf("expired bundle = %#v", expired)
	}
}

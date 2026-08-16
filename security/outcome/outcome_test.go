package outcome

import (
	"errors"
	"testing"
)

func TestClassifyHTTP(t *testing.T) {
	if ClassifyHTTP(200, nil) != Success {
		t.Fatal("200")
	}
	if ClassifyHTTP(400, nil) != DefiniteFailure {
		t.Fatal("400")
	}
	if ClassifyHTTP(500, nil) != Ambiguous {
		t.Fatal("500")
	}
	if ClassifyHTTP(0, errors.New("context deadline exceeded")) != Ambiguous {
		t.Fatal("timeout")
	}
}

package pass

import (
	"testing"
)

func TestParsing(t *testing.T) {
	_, err := GetPassTree("")
	if err != nil {
		t.Errorf("Error not nil: %s", err)
	}
}


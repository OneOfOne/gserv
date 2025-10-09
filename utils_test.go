package gserv

import (
	"testing"
)

func TestMatchStarOrigin(t *testing.T) {
	if !matchStarOrigin(nil, []string{"*.example.com"}, "1034.example.com") {
		t.Fatal("should match 1034.example.com")
	}
	if matchStarOrigin(nil, []string{"example.com"}, "1034.example.com") {
		t.Fatal("shouldn't match 1034.example.com")
	}
}

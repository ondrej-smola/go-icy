package icy_test

import (
	"testing"

	"github.com/noisypixy/go-icy"
)

func TestParseMetadata(t *testing.T) {
	rawMetadata := "StreamTitle='May'n - ViViD - ViViD';"

	expected := &icy.Metadata{
		StreamTitle: "May'n - ViViD - ViViD",
	}

	metadata, err := icy.ParseMetadata(rawMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if metadata == nil {
		t.Fatal("nil metadata")
	}
	if metadata.StreamTitle != expected.StreamTitle {
		t.Error("invalid StreamTitle: expecting %#v (got %#v)", metadata.StreamTitle, expected.StreamTitle)
	}
}

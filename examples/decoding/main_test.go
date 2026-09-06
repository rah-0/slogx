package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rah-0/slogx"
)

func TestRunPreservesOrderedRawAttributes(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	want := `INFO: item loaded
  item_id = 9007199254740993
  tag = "first"
  tag = "second"
  details = {"item":{"active":true}}
  msg = "application message"
  level = "application level"
WARN: retry scheduled
  attempt = 2
`
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestDecodeRecordsDistinguishesCleanEOFAndIncompleteInput(t *testing.T) {
	for _, input := range []string{"", " \n\t", sampleRecords} {
		if err := decodeRecords(strings.NewReader(input), io.Discard); err != nil {
			t.Fatalf("complete input returned %v", err)
		}
	}
	for _, input := range []string{"{", `{"level":"INFO","msg":"incomplete","item":`, sampleRecords + `{"msg":"unfinished"`} {
		err := decodeRecords(strings.NewReader(input), io.Discard)
		if err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("incomplete input %q returned %v, want a non-EOF error", input, err)
		}
	}
}

func TestDecodeRecordsPreservesDecoderSentinels(t *testing.T) {
	for _, test := range []struct {
		input string
		want  error
	}{
		{input: `[]`, want: slogx.ErrJSONRecordNotObject},
		{input: `{"level":false,"msg":"invalid"}`, want: slogx.ErrJSONFieldDecode},
	} {
		if err := decodeRecords(strings.NewReader(test.input), io.Discard); !errors.Is(err, test.want) {
			t.Fatalf("input %s returned %v, want %v", test.input, err, test.want)
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDecodeRecordsReturnsWriteFailure(t *testing.T) {
	if err := run(failedWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write failure = %v, want %v", err, io.ErrClosedPipe)
	}
}

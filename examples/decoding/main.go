package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rah-0/slogx"
)

// Fields following the built-in msg remain ordered attributes, even when their
// names repeat or match built-in names. Numbers and groups stay raw JSON.
const sampleRecords = `{"time":"2026-01-02T03:04:05Z","level":"INFO","msg":"item loaded","item_id":9007199254740993,"tag":"first","tag":"second","details":{"item":{"active":true}},"msg":"application message","level":"application level"}
{"level":"WARN","msg":"retry scheduled","attempt":2}
`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output io.Writer) error {
	return decodeRecords(strings.NewReader(sampleRecords), output)
}

func decodeRecords(input io.Reader, output io.Writer) error {
	decoder := slogx.NewJSONDecoder(input)
	for {
		record, err := decoder.Decode()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "%s: %s\n", record.Level, record.Message); err != nil {
			return err
		}
		for _, attr := range record.Attributes {
			if _, err := fmt.Fprintf(output, "  %s = %s\n", attr.Key, attr.Value); err != nil {
				return err
			}
		}
	}
}

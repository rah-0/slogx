package slogx

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// JSONAttr is one structured attribute from a JSON log record.
type JSONAttr struct {
	Key   string
	Value json.RawMessage
}

// JSONRecord is one record produced by slog.JSONHandler.
// Attributes retains structured fields in their original order, including
// repeated fields and fields whose names match slog's built-in keys.
type JSONRecord struct {
	Time       string
	Level      Level
	Message    string
	Source     *Source
	Attributes []JSONAttr
}

// JSONDecoder reads the stream of records produced by slog.JSONHandler.
type JSONDecoder struct {
	decoder *json.Decoder
}

// NewJSONDecoder returns a decoder that reads JSON log records from input.
func NewJSONDecoder(input io.Reader) *JSONDecoder {
	return &JSONDecoder{decoder: json.NewDecoder(input)}
}

// Decode reads the next JSON log record. It returns io.EOF at the end of input.
func (decoder *JSONDecoder) Decode() (JSONRecord, error) {
	start, err := decoder.decoder.Token()
	if err != nil {
		return JSONRecord{}, err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return JSONRecord{}, fmt.Errorf("slogx: JSON record must be an object")
	}

	var record JSONRecord
	builtins := true
	for decoder.decoder.More() {
		nameToken, err := decoder.decoder.Token()
		if err != nil {
			return JSONRecord{}, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return JSONRecord{}, fmt.Errorf("slogx: JSON record field name must be a string")
		}

		var value json.RawMessage
		if err := decoder.decoder.Decode(&value); err != nil {
			return JSONRecord{}, err
		}

		var target any
		if builtins {
			switch name {
			case slog.TimeKey:
				target = &record.Time
			case slog.LevelKey:
				target = &record.Level
			case slog.MessageKey:
				target = &record.Message
				builtins = false
			case slog.SourceKey:
				target = &record.Source
			}
		}
		if target == nil {
			record.Attributes = append(record.Attributes, JSONAttr{Key: name, Value: value})
			continue
		}
		if err := json.Unmarshal(value, target); err != nil {
			return JSONRecord{}, fmt.Errorf("slogx: decode %q: %w", name, err)
		}
	}
	end, err := decoder.decoder.Token()
	if err != nil {
		return JSONRecord{}, err
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
		return JSONRecord{}, fmt.Errorf("slogx: JSON record is not an object")
	}
	return record, nil
}

package slogx

import (
	"encoding/json"
	"errors"
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
// Attributes retains fields not consumed as built-ins in their original order,
// including repeated names and raw JSON values. See [JSONDecoder] for field ordering.
type JSONRecord struct {
	Time       string
	Level      Level
	Message    string
	Source     *Source
	Attributes []JSONAttr
}

// JSONDecoder reads the stream of records produced by slog.JSONHandler.
// It decodes time, level, source, and the first msg as built-in fields. All fields
// after the first msg become Attributes, including names that match built-ins.
// Before msg, repeated built-in fields overwrite their earlier decoded values.
type JSONDecoder struct {
	decoder *json.Decoder
}

// NewJSONDecoder returns a decoder that reads JSON log records from input.
func NewJSONDecoder(input io.Reader) *JSONDecoder {
	return &JSONDecoder{decoder: json.NewDecoder(input)}
}

// Decode reads the next JSON log record. It returns io.EOF only between records;
// an incomplete record returns a non-EOF error.
// Non-object records return [ErrJSONRecordNotObject]. Invalid built-in fields
// wrap [ErrJSONFieldDecode] and the underlying decoding error.
func (decoder *JSONDecoder) Decode() (JSONRecord, error) {
	start, err := decoder.decoder.Token()
	if err != nil {
		return JSONRecord{}, err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return JSONRecord{}, ErrJSONRecordNotObject
	}

	record, err := decoder.readObject()
	// In Go 1.27 with GOEXPERIMENT=nojsonv2, input "{" makes the JSON decoder's
	// Token() consume the opening brace. Its More() method then returns false,
	// meaning no next object member is available; it does not verify that the
	// closing brace exists. The next Token() call returns io.EOF while looking
	// for that missing brace. Report io.ErrUnexpectedEOF for the incomplete
	// object, reserving io.EOF for the end of input between records.
	if errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return record, err
}

func (decoder *JSONDecoder) readObject() (JSONRecord, error) {
	var record JSONRecord
	builtins := true
	for decoder.decoder.More() {
		nameToken, err := decoder.decoder.Token()
		if err != nil {
			return JSONRecord{}, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return JSONRecord{}, ErrInvalidJSONFieldName
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
			return JSONRecord{}, fmt.Errorf(errJSONFieldDecodeFormat, ErrJSONFieldDecode, name, err)
		}
	}
	end, err := decoder.decoder.Token()
	if err != nil {
		return JSONRecord{}, err
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
		return JSONRecord{}, ErrJSONRecordNotObject
	}
	return record, nil
}

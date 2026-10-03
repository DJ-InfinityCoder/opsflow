package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"opsflow/backend/internal/model"
)

func validateCustomFields(schemas []model.TeamFieldSchema, rawFields map[string]json.RawMessage) (map[string]any, error) {
	if rawFields == nil {
		rawFields = map[string]json.RawMessage{}
	}
	definitions := make(map[string]model.TeamFieldSchema, len(schemas))
	for _, schema := range schemas {
		definitions[schema.FieldKey] = schema
	}
	for key := range rawFields {
		if _, exists := definitions[key]; !exists {
			return nil, NewAppError(KindValidation, "custom_fields contains an unknown field", map[string]any{"field": key})
		}
	}

	values := make(map[string]any, len(rawFields))
	for _, schema := range schemas {
		raw, exists := rawFields[schema.FieldKey]
		if !exists {
			if schema.Required {
				return nil, NewAppError(KindValidation, "required custom field is missing", map[string]any{"field": schema.FieldKey})
			}
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, NewAppError(KindValidation, "custom field contains invalid JSON", map[string]any{"field": schema.FieldKey})
		}
		if value == nil {
			if schema.Required {
				return nil, NewAppError(KindValidation, "required custom field cannot be null", map[string]any{"field": schema.FieldKey})
			}
			continue
		}
		if err := validateFieldValue(schema, value); err != nil {
			return nil, err
		}
		values[schema.FieldKey] = value
	}
	return values, nil
}

func validateFieldValue(schema model.TeamFieldSchema, value any) error {
	valid := false
	switch strings.ToLower(schema.Type) {
	case "text", "string":
		_, valid = value.(string)
	case "number":
		_, valid = value.(json.Number)
	case "integer":
		if number, ok := value.(json.Number); ok {
			_, err := number.Int64()
			valid = err == nil
		}
	case "boolean":
		_, valid = value.(bool)
	case "date":
		if date, ok := value.(string); ok {
			_, err := time.Parse("2006-01-02", date)
			valid = err == nil
		}
	case "email":
		if email, ok := value.(string); ok {
			_, err := mail.ParseAddress(email)
			valid = err == nil
		}
	case "url":
		if rawURL, ok := value.(string); ok {
			parsed, err := url.ParseRequestURI(rawURL)
			valid = err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
		}
	case "select":
		if _, ok := value.(string); ok {
			valid = optionContains(schema.Options, value)
		}
	case "multi_select":
		if selections, ok := value.([]any); ok {
			valid = len(selections) > 0
			for _, selection := range selections {
				if _, ok := selection.(string); !ok || !optionContains(schema.Options, selection) {
					valid = false
					break
				}
			}
		}
	default:
		return NewAppError(KindValidation, "team field schema uses an unsupported field type", map[string]any{"field": schema.FieldKey, "type": schema.Type})
	}
	if !valid {
		return NewAppError(KindValidation, fmt.Sprintf("custom field %q has an invalid value for type %q", schema.FieldKey, schema.Type), map[string]any{"field": schema.FieldKey})
	}
	return nil
}

func optionContains(rawOptions json.RawMessage, value any) bool {
	var options []any
	if err := json.Unmarshal(rawOptions, &options); err != nil {
		return false
	}
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

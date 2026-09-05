package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The public llmserver contract is model-neutral. Every model receives the
// same small function surface; the model ID changes capacity, latency and
// price, never the wire protocol or Alice's causal rules.
func cognitiveModelTools(source map[string]any) ([]map[string]any, string) {
	shape := source["parameters"].(map[string]any)
	fields := shape["properties"].(map[string]any)
	bindings := cognitiveMetadataBindings(source)
	tool := copyObject(source)
	parameters, properties := copyObject(shape), copyObject(fields)
	for key := range bindings {
		delete(properties, key)
	}
	required := []string{}
	for _, key := range shape["required"].([]string) {
		if _, bound := bindings[key]; !bound {
			required = append(required, key)
		}
	}
	parameters["required"] = required
	properties["action"] = fields["action"]
	parameters["properties"] = properties
	tool["parameters"] = parameters
	tool["name"] = "cognitive_commit"
	tool["strict"] = false
	tool["description"] = source["description"].(string) + " action 选择一种完整动作形态；已由现实唯一确定的元数据由内核绑定。"
	return []map[string]any{tool}, "cognitive_commit"
}

func copyObject(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// Bind only top-level facts whose value the canonical contract already
// uniquely determines. Action and every meaningful choice remain model-owned.
func cognitiveMetadataBindings(tool map[string]any) map[string]any {
	fields := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	bindings := map[string]any{}
	for key, value := range fields {
		if key != "action" {
			if fixed, ok := fixedSchemaValue(value.(map[string]any)); ok {
				bindings[key] = fixed
			}
		}
	}
	return bindings
}

func fixedSchemaValue(schema map[string]any) (any, bool) {
	if values, ok := schema["enum"].([]string); ok && len(values) == 1 {
		return values[0], true
	}
	switch schema["type"] {
	case "string":
		if numericEqual(schema["maxLength"], 0) {
			return "", true
		}
	case "array":
		if numericEqual(schema["maxItems"], 0) {
			return []any{}, true
		}
	case "number", "integer":
		if minimum, exists := schema["minimum"]; exists && numericEqual(minimum, schema["maximum"]) {
			return minimum, true
		}
	case "object":
		fields, fieldsOK := schema["properties"].(map[string]any)
		required, requiredOK := schema["required"].([]string)
		if !fieldsOK || !requiredOK || len(fields) != len(required) || schema["additionalProperties"] != false {
			break
		}
		result := map[string]any{}
		for key, value := range fields {
			fixed, ok := fixedSchemaValue(value.(map[string]any))
			if !ok {
				return nil, false
			}
			result[key] = fixed
		}
		return result, true
	}
	return nil, false
}

// llmserver exposes the same function API for native and emulated providers.
// Use the portable subset for every model and validate every result locally.
func modelFunctionTools(source []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(source))
	for _, original := range source {
		tool := copyObject(original)
		tool["strict"] = false
		tool["parameters"] = portableFunctionParameters(original["parameters"].(map[string]any))
		result = append(result, tool)
	}
	return result
}

// An array constrained to [] does not need an unreachable item grammar. This
// keeps the declaration portable without changing its accepted value set.
func portableFunctionParameters(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = portableSchemaValue(value)
	}
	if source["type"] == "array" && numericEqual(source["maxItems"], 0) {
		result["items"] = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	}
	return result
}

func portableSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return portableFunctionParameters(typed)
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = portableSchemaValue(typed[index])
		}
		return result
	default:
		return value
	}
}

func decodeModelCommit(name, arguments string, tools []map[string]any, canonical map[string]any) (CognitiveCommit, error) {
	var commit CognitiveCommit
	schema, err := declaredFunctionSchema(name, tools)
	if err != nil {
		return commit, err
	}
	generated, err := decodeJSONObject(arguments)
	if err != nil {
		return commit, err
	}
	if err := validateJSONSchema(schema, generated, "$arguments"); err != nil {
		return commit, fmt.Errorf("function arguments violate declared schema: %w", err)
	}
	for key, value := range cognitiveMetadataBindings(canonical) {
		if _, supplied := generated[key]; supplied {
			return commit, fmt.Errorf("bound metadata %q must not be generated", key)
		}
		generated[key] = value
	}
	if err := validateJSONSchema(canonical["parameters"].(map[string]any), generated, "$commit"); err != nil {
		return commit, fmt.Errorf("bound cognitive commit violates canonical schema: %w", err)
	}
	bound, err := json.Marshal(generated)
	if err != nil {
		return commit, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(bound)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&commit); err != nil {
		return commit, err
	}
	if name != "cognitive_commit" {
		return commit, fmt.Errorf("unexpected cognitive function %q", name)
	}
	return commit, nil
}

// validateFunctionCall is the single local trust boundary for every model and
// every Hominal function. The gateway transports intent; it never authorizes or
// repairs an action for the body.
func validateFunctionCall(call *functionCall, tools []map[string]any) error {
	if call == nil {
		return errors.New("model returned no function call")
	}
	schema, err := declaredFunctionSchema(call.Name, tools)
	if err != nil {
		return err
	}
	arguments, err := decodeJSONObject(call.Arguments)
	if err != nil {
		return err
	}
	return validateJSONSchema(schema, arguments, "$arguments")
}

func declaredFunctionSchema(name string, tools []map[string]any) (map[string]any, error) {
	for _, tool := range tools {
		if tool["name"] == name {
			schema, ok := tool["parameters"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("function %q has no local parameter schema", name)
			}
			return schema, nil
		}
	}
	return nil, fmt.Errorf("model returned undeclared function %q", name)
}

func decodeJSONObject(source string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		if err == nil {
			err = errors.New("value is not a JSON object")
		}
		return nil, fmt.Errorf("function arguments are invalid: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("function arguments must contain exactly one JSON object")
	}
	return value, nil
}

// validateJSONSchema intentionally implements only the small JSON Schema
// vocabulary emitted by Hominal. It is deterministic, fast and fail-closed;
// adding a new schema keyword requires extending this local validator first.
func validateJSONSchema(schema map[string]any, value any, path string) error {
	if alternatives, ok := schema["anyOf"].([]any); ok {
		for _, candidate := range alternatives {
			if branch, ok := candidate.(map[string]any); ok && validateJSONSchema(branch, value, path) == nil {
				return nil
			}
		}
		return fmt.Errorf("%s does not match any permitted shape", path)
	}
	if enum, exists := schema["enum"]; exists && !schemaEnumContains(enum, value) {
		return fmt.Errorf("%s is not an allowed value", path)
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		for _, key := range schemaStringList(schema["required"]) {
			if _, exists := object[key]; !exists {
				return fmt.Errorf("%s.%s is required", path, key)
			}
		}
		for key, item := range object {
			child, exists := properties[key]
			if !exists {
				if schema["additionalProperties"] == false {
					return fmt.Errorf("%s.%s is not declared", path, key)
				}
				continue
			}
			if err := validateJSONSchema(child.(map[string]any), item, path+"."+key); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if minimum, ok := schemaNumber(schema["minItems"]); ok && float64(len(array)) < minimum {
			return fmt.Errorf("%s has too few items", path)
		}
		if maximum, ok := schemaNumber(schema["maxItems"]); ok && float64(len(array)) > maximum {
			return fmt.Errorf("%s has too many items", path)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for index, item := range array {
				if err := validateJSONSchema(itemSchema, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		length := float64(utf8.RuneCountInString(text))
		if minimum, ok := schemaNumber(schema["minLength"]); ok && length < minimum {
			return fmt.Errorf("%s is too short", path)
		}
		if maximum, ok := schemaNumber(schema["maxLength"]); ok && length > maximum {
			return fmt.Errorf("%s is too long", path)
		}
		if pattern, ok := schema["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, text)
			if err != nil || !matched {
				return fmt.Errorf("%s does not match its required pattern", path)
			}
		}
	case "number", "integer":
		number, ok := schemaNumber(value)
		if !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		if schema["type"] == "integer" && number != math.Trunc(number) {
			return fmt.Errorf("%s must be an integer", path)
		}
		if minimum, ok := schemaNumber(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("%s is below its minimum", path)
		}
		if maximum, ok := schemaNumber(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("%s exceeds its maximum", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case nil:
		// anyOf-only schemas are handled above. Hominal emits no unconstrained
		// data-bearing schema; this case is retained for empty unreachable nodes.
	default:
		return fmt.Errorf("%s uses an unsupported local schema type", path)
	}
	return nil
}

func schemaStringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func schemaEnumContains(enum, value any) bool {
	switch values := enum.(type) {
	case []string:
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, candidate := range values {
			if text == candidate {
				return true
			}
		}
	case []any:
		for _, candidate := range values {
			if reflect.DeepEqual(candidate, value) || numericEqual(candidate, value) {
				return true
			}
		}
	}
	return false
}

func numericEqual(left, right any) bool {
	a, aOK := schemaNumber(left)
	b, bOK := schemaNumber(right)
	return aOK && bOK && a == b
}

func schemaNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		result, err := number.Float64()
		return result, err == nil
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint64:
		return float64(number), true
	default:
		return 0, false
	}
}

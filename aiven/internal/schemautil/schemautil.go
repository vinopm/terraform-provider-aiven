// Copyright (c) 2017 jelmersnoeck
// Copyright (c) 2018-2021 Aiven, Helsinki, Finland. https://aiven.io/
package schemautil

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func OptionalString(d *schema.ResourceData, key string) string {
	str, ok := d.Get(key).(string)
	if !ok {
		return ""
	}
	return str
}

// OptionalStringPointer retrieves a string pointer to a field, empty string
// will be converted to nil
func OptionalStringPointer(d *schema.ResourceData, key string) *string {
	val, ok := d.GetOk(key)
	if !ok {
		return nil
	}
	str, ok := val.(string)
	if !ok {
		return nil
	}
	return &str
}

// OptionalStringPointerForUndefined retrieves a string pointer to a field, empty
// string remains a pointer to an empty string
func OptionalStringPointerForUndefined(d *schema.ResourceData, key string) *string {
	str, ok := d.Get(key).(string)
	if !ok {
		return nil
	}
	return &str
}

func OptionalIntPointer(d *schema.ResourceData, key string) *int {
	val, ok := d.GetOk(key)
	if !ok {
		return nil
	}
	intValue, ok := val.(int)
	if !ok {
		return nil
	}
	return &intValue
}

func ToOptionalString(val interface{}) string {
	switch v := val.(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case string:
		return v
	default:
		return ""
	}
}

func ParseOptionalStringToInt64(val interface{}) *int64 {
	v, ok := val.(string)
	if !ok {
		return nil
	}

	if v == "" {
		return nil
	}

	res, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}

	return &res
}

func ParseOptionalStringToFloat64(val interface{}) *float64 {
	v, ok := val.(string)
	if !ok {
		return nil
	}

	if v == "" {
		return nil
	}

	res, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}

	return &res
}

func ParseOptionalStringToBool(val interface{}) *bool {
	v, ok := val.(string)
	if !ok {
		return nil
	}

	if v == "" {
		return nil
	}

	res, err := strconv.ParseBool(v)
	if err != nil {
		return nil
	}

	return &res
}

func CreateOnlyDiffSuppressFunc(_, _, _ string, d *schema.ResourceData) bool {
	return len(d.Id()) > 0
}

func DiskSpaceDiffSuppressFunc(_, o, n string, d *schema.ResourceData) bool {
	// check if new is empty; if so get default from schema ( `disk_space_default` )
	if n == "" {
		// default is required field in the api and should be there once we can diff the resource
		// and it was created
		n = d.Get("disk_space_default").(string)
	}

	nb, _ := units.RAMInBytes(n)
	ob, _ := units.RAMInBytes(o)
	return nb == ob
}

// EmptyObjectDiffSuppressFunc suppresses a diff for service user configuration options when
// fields are not set by the user but have default or previously defined values.
func EmptyObjectDiffSuppressFunc(k, old, new string, _ *schema.ResourceData) bool {
	// When a map inside a list contains only default values without explicit values set by
	// the user Terraform interprets the map as not being present and the array length being
	// zero, resulting in bogus update that does nothing. Allow ignoring those.
	if old == "1" && new == "0" && strings.HasSuffix(k, ".#") {
		return true
	}

	// When a field is not set to any value and consequently is null (empty string) but had
	// a non-empty parameter before. Allow ignoring those.
	if new == "" && old != "" {
		return true
	}

	// There is a bug in Terraform 0.11 which interprets "true" as "0" and "false" as "1"
	if (new == "0" && old == "false") || (new == "1" && old == "true") {
		return true
	}

	return false
}

// EmptyObjectDiffSuppressFuncSkipArrays generates a DiffSuppressFunc that skips all the array/list fields
// and uses schemautil.EmptyObjectDiffSuppressFunc in all others cases
func EmptyObjectDiffSuppressFuncSkipArrays(s map[string]*schema.Schema) schema.SchemaDiffSuppressFunc {
	var skipKeys []string
	for key, sh := range s {
		if sh.Type == schema.TypeList {
			skipKeys = append(skipKeys, key)
		}
	}

	return func(k, old, new string, d *schema.ResourceData) bool {
		for _, key := range skipKeys {
			if strings.Contains(k, fmt.Sprintf(".%s.", key)) {
				return false
			}
		}

		return EmptyObjectDiffSuppressFunc(k, old, new, d)
	}
}

// EmptyObjectNoChangeDiffSuppressFunc it suppresses a diff if a field is empty but have not
// been set before to any value
func EmptyObjectNoChangeDiffSuppressFunc(k, _, new string, d *schema.ResourceData) bool {
	if d.HasChange(k) {
		return false
	}

	if new == "" {
		return true
	}

	return false
}

// Terraform does not allow default values for arrays but the IP filter user config value
// has default. We don't want to force users to always define explicit value just because
// of the Terraform restriction so suppress the change from default to empty (which would
// be nonsensical operation anyway)
func IpFilterArrayDiffSuppressFunc(k, old, new string, d *schema.ResourceData) bool {
	if old == "1" && new == "0" && strings.HasSuffix(k, ".ip_filter.#") {
		if list, ok := d.Get(strings.TrimSuffix(k, ".#")).([]interface{}); ok {
			if len(list) == 1 {
				return list[0] == "0.0.0.0/0"
			}
		}
	}

	return false
}

func IpFilterValueDiffSuppressFunc(k, old, new string, _ *schema.ResourceData) bool {
	return old == "0.0.0.0/0" && new == "" && strings.HasSuffix(k, ".ip_filter.0")
}

// ValidateDurationString is a ValidateFunc that ensures a string parses
// as time.Duration format
func ValidateDurationString(v interface{}, k string) (ws []string, errors []error) {
	if _, err := time.ParseDuration(v.(string)); err != nil {
		errors = append(errors, fmt.Errorf("%q: invalid duration", k))
	}
	return
}

// schemautil.ValidateHumanByteSizeString is a ValidateFunc that ensures a string parses
// as units.Bytes format
func ValidateHumanByteSizeString(v interface{}, k string) (ws []string, errors []error) {
	// only allow `^[1-9][0-9]*(GiB|G)*` without fractions
	if ok, _ := regexp.MatchString("^[1-9][0-9]*(GiB|G)$", v.(string)); !ok {
		return ws, append(errors, fmt.Errorf("%q: configured string must match ^[1-9][0-9]*(G|GiB)", k))
	}
	if _, err := units.RAMInBytes(v.(string)); err != nil {
		return ws, append(errors, fmt.Errorf("%q: invalid human readable byte size", k))
	}
	return
}

func BuildResourceID(parts ...string) string {
	finalParts := make([]string, len(parts))
	for idx, part := range parts {
		finalParts[idx] = url.PathEscape(part)
	}
	return strings.Join(finalParts, "/")
}

func splitResourceID(resourceID string, n int) []string {
	parts := strings.SplitN(resourceID, "/", n)
	for idx, part := range parts {
		part, _ := url.PathUnescape(part)
		parts[idx] = part
	}
	return parts
}

func SplitResourceID2(resourceID string) (string, string) {
	parts := splitResourceID(resourceID, 2)
	return parts[0], parts[1]
}

func SplitResourceID3(resourceID string) (string, string, string) {
	parts := splitResourceID(resourceID, 3)
	return parts[0], parts[1], parts[2]
}

func SplitResourceID4(resourceID string) (string, string, string, string) {
	parts := splitResourceID(resourceID, 4)
	return parts[0], parts[1], parts[2], parts[3]
}

func FlattenToString(a []interface{}) []string {
	r := make([]string, len(a))
	for i, v := range a {
		r[i] = fmt.Sprint(v)
	}

	return r
}

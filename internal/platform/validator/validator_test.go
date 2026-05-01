package validator_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type testStruct struct {
	Name  string `validate:"required,min=2,max=50"`
	Email string `validate:"required,email"`
	Role  string `validate:"required,oneof=admin user"`
}

func TestValidator_ValidStruct(t *testing.T) {
	v := validator.New()
	s := testStruct{Name: "John", Email: "john@example.com", Role: "admin"}

	errs := v.Validate(s)
	assert.Nil(t, errs)
}

func TestValidator_RequiredFieldMissing(t *testing.T) {
	v := validator.New()
	s := testStruct{}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "this field is required", errs["name"])
	assert.Equal(t, "this field is required", errs["email"])
	assert.Equal(t, "this field is required", errs["role"])
}

func TestValidator_InvalidEmail(t *testing.T) {
	v := validator.New()
	s := testStruct{Name: "John", Email: "not-an-email", Role: "user"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be a valid email address", errs["email"])
}

func TestValidator_MinLength(t *testing.T) {
	v := validator.New()
	s := testStruct{Name: "J", Email: "john@example.com", Role: "user"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be at least 2 characters", errs["name"])
}

func TestValidator_MaxLength(t *testing.T) {
	v := validator.New()
	longName := ""
	var longNameSb57 strings.Builder
	for range 51 {
		longNameSb57.WriteString("a")
	}
	longName += longNameSb57.String()
	s := testStruct{Name: longName, Email: "john@example.com", Role: "user"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be at most 50 characters", errs["name"])
}

func TestValidator_OneOf(t *testing.T) {
	v := validator.New()
	s := testStruct{Name: "John", Email: "john@example.com", Role: "moderator"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be one of: admin user", errs["role"])
}

type uuidStruct struct {
	ID string `validate:"required,uuid"`
}

func TestValidator_UUID(t *testing.T) {
	v := validator.New()
	s := uuidStruct{ID: "not-a-uuid"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be a valid UUID", errs["iD"])
}

type urlStruct struct {
	Website string `validate:"required,url"`
}

func TestValidator_URL(t *testing.T) {
	v := validator.New()
	s := urlStruct{Website: "not-a-url"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be a valid URL", errs["website"])
}

type gteStruct struct {
	Age int `validate:"required,gte=18"`
}

func TestValidator_GTE(t *testing.T) {
	v := validator.New()
	s := gteStruct{Age: 10}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be greater than or equal to 18", errs["age"])
}

type lteStruct struct {
	Score int `validate:"required,lte=100"`
}

func TestValidator_LTE(t *testing.T) {
	v := validator.New()
	s := lteStruct{Score: 150}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "must be less than or equal to 100", errs["score"])
}

type defaultTagStruct struct {
	Value string `validate:"required,alphanum"`
}

func TestValidator_DefaultTag(t *testing.T) {
	v := validator.New()
	s := defaultTagStruct{Value: "hello world!"}

	errs := v.Validate(s)
	require.NotNil(t, errs)
	assert.Equal(t, "failed on alphanum validation", errs["value"])
}

func TestValidator_NonStructInput(t *testing.T) {
	v := validator.New()
	errs := v.Validate("not a struct")
	require.NotNil(t, errs)
	assert.Contains(t, errs, "error")
}

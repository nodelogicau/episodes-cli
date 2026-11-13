package model

import (
	"fmt"
	"strings"
)

// Severity of a finding.
type Severity string

// Severities, most severe first.
const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
	SevInfo    Severity = "info"
)

// Problem is one finding about one episode. Writers refuse on any error;
// validate reports every problem.
type Problem struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Field    string   `json:"field,omitempty"`
	Message  string   `json:"message"`
}

func (p Problem) String() string {
	if p.Field != "" {
		return fmt.Sprintf("%s (%s): %s", p.Code, p.Field, p.Message)
	}
	return fmt.Sprintf("%s: %s", p.Code, p.Message)
}

// Problems is a list of findings.
type Problems []Problem

// Errors returns the error-severity problems.
func (ps Problems) Errors() Problems {
	var out Problems
	for _, p := range ps {
		if p.Severity == SevError {
			out = append(out, p)
		}
	}
	return out
}

// Error joins the problems, so a refusal names every rule it broke.
func (ps Problems) Error() string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return strings.Join(parts, "; ")
}

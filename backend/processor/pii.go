package processor

import (
	"regexp"

	"github.com/alma/assignment/models"
)

// PIIDetector matches a regex pattern to detect a specific PII type in text.
type PIIDetector struct {
	Type    models.PIIType
	Pattern *regexp.Regexp
}

// DefaultPIIDetectors returns the built-in set of PII detectors.
func DefaultPIIDetectors() []PIIDetector {
	return []PIIDetector{
		{Type: models.PIITypeEmail, Pattern: regexp.MustCompile(`\b[\w.%+\-]+@[\w.\-]+\.[a-zA-Z]{2,}\b`)},
		{Type: models.PIITypeCreditCard, Pattern: regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`)},
		{Type: models.PIITypeSSN, Pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
		{Type: models.PIITypePhone, Pattern: regexp.MustCompile(`\b(\+\d{1,3}[- ]?)?\(?\d{3}\)?[- ]?\d{3}[- ]?\d{4}\b`)},
		{Type: models.PIITypeIPAddress, Pattern: regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)},
	}
}

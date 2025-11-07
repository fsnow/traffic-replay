package sender

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestCleanInternalFields(t *testing.T) {
	tests := []struct {
		name     string
		input    bson.M
		expected bson.M
	}{
		{
			name: "removes $clusterTime",
			input: bson.M{
				"insert":       "users",
				"$clusterTime": bson.M{"clusterTime": "12345"},
				"documents": bson.A{
					bson.M{"name": "Alice"},
				},
			},
			expected: bson.M{
				"insert": "users",
				"documents": bson.A{
					bson.M{"name": "Alice"},
				},
			},
		},
		{
			name: "removes lsid and txnNumber",
			input: bson.M{
				"find":      "users",
				"filter":    bson.M{"age": 30},
				"lsid":      bson.M{"id": "session123"},
				"txnNumber": int64(5),
			},
			expected: bson.M{
				"find":   "users",
				"filter": bson.M{"age": 30},
			},
		},
		{
			name: "preserves MongoDB operators",
			input: bson.M{
				"update": "users",
				"updates": bson.A{
					bson.M{
						"q": bson.M{"name": "Alice"},
						"u": bson.M{
							"$set": bson.M{"age": 31},
							"$inc": bson.M{"loginCount": 1},
						},
					},
				},
				"$clusterTime": bson.M{"clusterTime": "12345"},
			},
			expected: bson.M{
				"update": "users",
				"updates": bson.A{
					bson.M{
						"q": bson.M{"name": "Alice"},
						"u": bson.M{
							"$set": bson.M{"age": 31},
							"$inc": bson.M{"loginCount": 1},
						},
					},
				},
			},
		},
		{
			name: "removes $db field",
			input: bson.M{
				"insert": "users",
				"$db":    "testdb",
				"documents": bson.A{
					bson.M{"name": "Bob"},
				},
			},
			expected: bson.M{
				"insert": "users",
				"documents": bson.A{
					bson.M{"name": "Bob"},
				},
			},
		},
		{
			name: "handles nested documents",
			input: bson.M{
				"aggregate": "orders",
				"pipeline": bson.A{
					bson.M{
						"$lookup": bson.M{
							"from":         "customers",
							"localField":   "customerId",
							"foreignField": "_id",
							"as":           "customer",
						},
					},
				},
				"$clusterTime": bson.M{"time": "12345"},
				"lsid":         bson.M{"id": "session456"},
			},
			expected: bson.M{
				"aggregate": "orders",
				"pipeline": bson.A{
					bson.M{
						"$lookup": bson.M{
							"from":         "customers",
							"localField":   "customerId",
							"foreignField": "_id",
							"as":           "customer",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanInternalFields(tt.input)

			// Compare the results
			if !bsonEqual(result, tt.expected) {
				t.Errorf("cleanInternalFields() mismatch\nGot:      %v\nExpected: %v", result, tt.expected)
			}
		})
	}
}

// bsonEqual compares two bson.M documents for equality
// This is a simplified comparison for testing purposes
func bsonEqual(a, b bson.M) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, exists := b[key]
		if !exists {
			return false
		}

		// Compare based on type
		switch aTyped := aVal.(type) {
		case bson.M:
			if bTyped, ok := bVal.(bson.M); ok {
				if !bsonEqual(aTyped, bTyped) {
					return false
				}
			} else {
				return false
			}
		case bson.A:
			if bTyped, ok := bVal.(bson.A); ok {
				if !bsonArrayEqual(aTyped, bTyped) {
					return false
				}
			} else {
				return false
			}
		default:
			// For primitive types, use simple equality
			if aVal != bVal {
				return false
			}
		}
	}

	return true
}

// bsonArrayEqual compares two bson.A arrays for equality
func bsonArrayEqual(a, b bson.A) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		switch aTyped := a[i].(type) {
		case bson.M:
			if bTyped, ok := b[i].(bson.M); ok {
				if !bsonEqual(aTyped, bTyped) {
					return false
				}
			} else {
				return false
			}
		case bson.A:
			if bTyped, ok := b[i].(bson.A); ok {
				if !bsonArrayEqual(aTyped, bTyped) {
					return false
				}
			} else {
				return false
			}
		default:
			if a[i] != b[i] {
				return false
			}
		}
	}

	return true
}

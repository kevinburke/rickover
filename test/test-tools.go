// Copyright 2014 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// Copied from https://github.com/letsencrypt/boulder/blob/master/test/test-tools.go
//
// See Q5 and Q11 here: https://www.mozilla.org/en-US/MPL/2.0/FAQ/ I think if
// we want to modify this file we have to release it publicly, otherwise we're
// fine.
package test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Assert a boolean
func Assert(t testing.TB, result bool, message string) {
	t.Helper()
	if !result {
		t.Fatal(message)
	}
}

// AssertNotNil checks an object to be non-nil
func AssertNotNil(t testing.TB, obj interface{}, message string) {
	t.Helper()
	if obj == nil {
		t.Fatal(message)
	}
}

// AssertNotError checks that err is nil
func AssertNotError(t testing.TB, err error, message string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %s", message, err)
	}
}

// AssertError checks that err is non-nil
func AssertError(t testing.TB, err error, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but received none", message)
	}
}

// AssertEquals uses the equality operator (==) to measure one and two
func AssertEquals(t testing.TB, one interface{}, two interface{}) {
	t.Helper()
	if one != two {
		t.Fatalf("[%v] != [%v]", one, two)
	}
}

// AssertDeepEquals uses the reflect.DeepEqual method to measure one and two
func AssertDeepEquals(t testing.TB, one interface{}, two interface{}) {
	t.Helper()
	if !reflect.DeepEqual(one, two) {
		t.Fatalf("[%+v] !(deep)= [%+v]", one, two)
	}
}

// AssertMarshaledEquals marshals one and two to JSON, and then uses
// the equality operator to measure them
func AssertMarshaledEquals(t testing.TB, one interface{}, two interface{}) {
	t.Helper()
	oneJSON, err := json.Marshal(one)
	AssertNotError(t, err, "Could not marshal 1st argument")
	twoJSON, err := json.Marshal(two)
	AssertNotError(t, err, "Could not marshal 2nd argument")

	if !bytes.Equal(oneJSON, twoJSON) {
		t.Fatalf("[%s] !(json)= [%s]", oneJSON, twoJSON)
	}
}

// AssertNotEquals uses the equality operator to measure that one and two
// are different
func AssertNotEquals(t testing.TB, one interface{}, two interface{}) {
	t.Helper()
	if one == two {
		t.Fatalf("[%v] == [%v]", one, two)
	}
}

// AssertByteEquals uses bytes.Equal to measure one and two for equality.
func AssertByteEquals(t testing.TB, one []byte, two []byte) {
	t.Helper()
	if !bytes.Equal(one, two) {
		t.Fatalf("Byte [%s] != [%s]",
			base64.StdEncoding.EncodeToString(one),
			base64.StdEncoding.EncodeToString(two))
	}
}

// AssertIntEquals uses the equality operator to measure one and two.
func AssertIntEquals(t testing.TB, one int, two int) {
	t.Helper()
	if one != two {
		t.Fatalf("Int [%d] != [%d]", one, two)
	}
}

// AssertContains determines whether needle can be found in haystack
func AssertContains(t testing.TB, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("String [%s] does not contain [%s]", haystack, needle)
	}
}

// AssertNotContains determines if needle is not found in haystack
func AssertNotContains(t testing.TB, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("String [%s] contains [%s]", haystack, needle)
	}
}

// AssertSeverity determines if a string matches the Severity formatting
func AssertSeverity(t testing.TB, data string, severity int) {
	t.Helper()
	expected := fmt.Sprintf("\"severity\":%d", severity)
	AssertContains(t, data, expected)
}

// AssertBetween determines if a is between b and c
func AssertBetween(t testing.TB, a, b, c int64) {
	t.Helper()
	if a < b || a > c {
		t.Fatalf("%d is not between %d and %d", a, b, c)
	}
}

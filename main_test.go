package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPushMessage(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(`PUT /{queue}`, PushMessage)

	testCases := []struct {
		name               string
		qName              string
		v                  string
		expectedStatusCode int
		checkQ             bool
	}{
		{
			name:               "v provided & queue isn't empty",
			qName:              "queue",
			v:                  "elem",
			expectedStatusCode: http.StatusOK,
			checkQ:             true,
		},
		{
			name:               "v isn't provided",
			qName:              "queue",
			expectedStatusCode: http.StatusBadRequest,
			checkQ:             false,
		},
		{
			name:               "queue is empty",
			v:                  "queue",
			expectedStatusCode: http.StatusNotFound,
			checkQ:             false,
		},
	}

	for idx, testCase := range testCases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", pushEndpoint(testCase.qName, testCase.v), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != testCase.expectedStatusCode {
			t.Errorf(
				"#%d %s: incorrect status code: got: %d, expected: %d",
				idx,
				testCase.name,
				rec.Result().StatusCode,
				testCase.expectedStatusCode,
			)

			continue
		}

		if testCase.checkQ {
			q, exists := qStor.storage[testCase.qName]
			if !exists {
				t.Errorf(
					"#%d %s: expected queue with name %q to exist",
					idx,
					testCase.name,
					testCase.qName,
				)

				continue
			}

			if q.head == nil || q.head.value != testCase.v {
				t.Errorf(
					"#%d %s: expected queue head to have value %q",
					idx,
					testCase.name,
					testCase.v,
				)

				continue
			}

			if q.head.next != nil {
				t.Errorf(
					"#%d %s: expected queue to have only one value",
					idx,
					testCase.name,
				)

				continue
			}
		}
	}
}

func TestPopMessage(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(`GET /{queue}`, PopMessage)

	// test non-existent queue
	{
		prefix := "non-existent queue"
		qName := "non-existent"
		delete(qStor.storage, qName) // for the purity of the test

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, ""), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 404",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test empty queue
	{
		prefix := "empty queue"
		qName := "empty"
		// emptyQ := &Queue{head: nil, tail: nil}
		emptyQ := NewQueue("")
		qStor.storage[qName] = emptyQ
		defer func() {
			delete(qStor.storage, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, ""), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 404",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test one element queue
	{
		prefix := "one element queue"
		qName := "1el"
		val := "one el q val"
		// qEl := &queueElement{value: "one el q val", next: nil}
		// q := &Queue{head: qEl, tail: qEl}
		q := NewQueue(val)
		qStor.storage[qName] = q
		defer func() {
			delete(qStor.storage, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, ""), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusOK {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 200",
				prefix,
				rec.Result().StatusCode,
			)
		}

		if rec.Body.String() != val {
			t.Fatalf(
				"%s: incorrect output body: got: %q, expected: %q",
				prefix,
				rec.Body.String(),
				val,
			)
		}

		if q.head != nil || q.tail != nil {
			t.Fatalf(
				"%s: expected q to become empty",
				prefix,
			)
		}
	}

	// test two element queue
	{
		prefix := "two element queue"
		qName := "2el"
		val1 := "two el q val 1"
		val2 := "two el q val 2"
		// qEl2 := &queueElement{value: "two el q val 2", next: nil}
		// qEl1 := &queueElement{value: "two el q val 1", next: qEl2}
		// q := &Queue{head: qEl1, tail: qEl2}
		q := NewQueue(val1)
		q.Push(val2)
		qStor.storage[qName] = q
		defer func() {
			delete(qStor.storage, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, ""), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusOK {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 200",
				prefix,
				rec.Result().StatusCode,
			)
		}

		if rec.Body.String() != val1 {
			t.Fatalf(
				"%s: incorrect output body: got: %q, expected: %q",
				prefix,
				rec.Body.String(),
				val1,
			)
		}

		if q.head.value != val2 || q.tail.value != val2 {
			t.Fatalf(
				"%s: expected element to be removed and one left",
				prefix,
			)
		}
	}

	// test non-numeric timeout
	{
		prefix := "non-numeric timeout"
		qName := "empty"
		timeout := "notanum"

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusBadRequest {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 400",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test negative timeout
	{
		prefix := "negative timeout"
		qName := "empty"
		timeout := "-3"

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusBadRequest {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 400",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test 0 timeout and empty queue
	// 0 timeout acts like no timeout provided
	{
		prefix := "0 timeout"
		qName := "empty"
		timeout := "0"

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 404",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test req with timeout to an empty queue
	{
		prefix := "req with timeout to an empty queue"
		qName := "empty"
		timeout := "1"

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusNotFound {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 404",
				prefix,
				rec.Result().StatusCode,
			)
		}
	}

	// test req with timeout to non-empty queue
	{
		prefix := "req with timeout to an empty queue"
		qName := "non-empty"
		timeout := "1"
		expectedVal := "one el q val"
		// qEl := &queueElement{value: expectedVal, next: nil}
		// q := &Queue{head: qEl, tail: qEl}
		q := NewQueue(expectedVal)
		qStor.storage[qName] = q
		defer func() {
			delete(qStor.storage, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusOK {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 200",
				prefix,
				rec.Result().StatusCode,
			)
		}

		if rec.Body.String() != expectedVal {
			t.Fatalf(
				"%s: incorrect output body: got: %q, expected: %q",
				prefix,
				rec.Body.String(),
				expectedVal,
			)
		}
	}

	// test req with timeout to an empty queue that receives val in the middle of awaiting
	{
		prefix := "req with timeout to an empty queue that receives val in the middle of awaiting"
		qName := "empty-but-potentially-not"
		timeout := "2"
		// q := &Queue{}
		q := NewQueue("")
		qStor.storage[qName] = q
		defer func() {
			delete(qStor.storage, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName, timeout), nil)

		signalDone := make(chan struct{})

		go func() {
			mux.ServeHTTP(rec, req)

			signalDone <- struct{}{}
		}()

		time.Sleep(2 * time.Second)
		expectedVal := "val"
		q.Push(expectedVal)

		select {
		case <-time.After(1 * time.Second):
			t.Fatalf(
				"%s: time exceeded, but res hasn't been received",
				prefix,
			)
		case <-signalDone:
			if rec.Result().StatusCode != http.StatusOK {
				t.Fatalf(
					"%s: incorrect status code: got: %d, expected: 200",
					prefix,
					rec.Result().StatusCode,
				)
			}

			if rec.Body.String() != expectedVal {
				t.Fatalf(
					"%s: incorrect output body: got: %q, expected: %q",
					prefix,
					rec.Body.String(),
					expectedVal,
				)
			}
		}
	}
}

func TestPushAndPopMessage(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc(`PUT /{queue}`, PushMessage)
	mux.HandleFunc(`GET /{queue}`, PopMessage)

	testCases := []struct {
		name               string
		method             string
		path               string
		expectedStatusCode int
		expectedOutput     string
	}{
		{
			name:               "cat => pet",
			method:             "PUT",
			path:               "/pet?v=cat",
			expectedStatusCode: 200,
		},
		{
			name:               "dog => pet",
			method:             "PUT",
			path:               "/pet?v=dog",
			expectedStatusCode: 200,
		},
		{
			name:               "manager => role",
			method:             "PUT",
			path:               "/role?v=manager",
			expectedStatusCode: 200,
		},
		{
			name:               "executive => role",
			method:             "PUT",
			path:               "/role?v=executive",
			expectedStatusCode: 200,
		},
		{
			name:               "<= pet",
			method:             "GET",
			path:               "/pet",
			expectedStatusCode: 200,
			expectedOutput:     "cat",
		},
		{
			name:               "<= pet",
			method:             "GET",
			path:               "/pet",
			expectedStatusCode: 200,
			expectedOutput:     "dog",
		},
		{
			name:               "<= pet",
			method:             "GET",
			path:               "/pet",
			expectedStatusCode: 404,
		},
		{
			name:               "<= pet",
			method:             "GET",
			path:               "/pet",
			expectedStatusCode: 404,
		},
		{
			name:               "<= role",
			method:             "GET",
			path:               "/role",
			expectedStatusCode: 200,
			expectedOutput:     "manager",
		},
		{
			name:               "<= role",
			method:             "GET",
			path:               "/role",
			expectedStatusCode: 200,
			expectedOutput:     "executive",
		},
		{
			name:               "<= role",
			method:             "GET",
			path:               "/role",
			expectedStatusCode: 404,
		},
	}

	for idx, testCase := range testCases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(testCase.method, testCase.path, nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != testCase.expectedStatusCode {
			t.Errorf(
				"#%d %s: incorrect status code: got: %d, expected: %d",
				idx,
				testCase.name,
				rec.Result().StatusCode,
				testCase.expectedStatusCode,
			)

			continue
		}

		if testCase.expectedOutput != "" {
			if rec.Body.String() != testCase.expectedOutput {
				t.Errorf(
					"#%d %s: incorrect response body: got: %s, expected: %s",
					idx,
					testCase.name,
					rec.Body.String(),
					testCase.expectedOutput,
				)

				continue
			}
		}
	}
}

func pushEndpoint(qName, v string) string {
	endpoint := "/" + qName
	if v != "" {
		endpoint += "?v=" + v
	}

	return endpoint
}

func popEndpoint(qName, timeout string) string {
	endpoint := "/" + qName
	if timeout != "" {
		endpoint += "?timeout=" + timeout
	}

	return endpoint
}

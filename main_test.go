package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
			q, exists := qStor[testCase.qName]
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
		delete(qStor, qName) // for the purity of the test

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName), nil)

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
		emptyQ := &queue{head: nil, tail: nil}
		qStor[qName] = emptyQ
		defer func() {
			delete(qStor, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName), nil)

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
		qEl := &queueElement{value: "one el q val", next: nil}
		q := &queue{head: qEl, tail: qEl}
		qStor[qName] = q
		defer func() {
			delete(qStor, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusOK {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 200",
				prefix,
				rec.Result().StatusCode,
			)
		}

		if rec.Body.String() != qEl.value {
			t.Fatalf(
				"%s: incorrect output body: got: %q, expected: %q",
				prefix,
				rec.Body.String(),
				qEl.value,
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
		qEl2 := &queueElement{value: "two el q val 2", next: nil}
		qEl1 := &queueElement{value: "two el q val 1", next: qEl2}
		q := &queue{head: qEl1, tail: qEl2}
		qStor[qName] = q
		defer func() {
			delete(qStor, qName)
		}()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", popEndpoint(qName), nil)

		mux.ServeHTTP(rec, req)

		if rec.Result().StatusCode != http.StatusOK {
			t.Fatalf(
				"%s: incorrect status code: got: %d, expected: 200",
				prefix,
				rec.Result().StatusCode,
			)
		}

		if rec.Body.String() != qEl1.value {
			t.Fatalf(
				"%s: incorrect output body: got: %q, expected: %q",
				prefix,
				rec.Body.String(),
				qEl1.value,
			)
		}

		if q.head != qEl2 || q.tail != qEl2 {
			t.Fatalf(
				"%s: expected element to be removed and one left",
				prefix,
			)
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

func popEndpoint(qName string) string {
	return "/" + qName
}

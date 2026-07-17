package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{queue_name}", popMessage)
	mux.HandleFunc("PUT /{queue_name}", pushMessage)

	reqPop := func(q, timeout string) *http.Request {
		path := "/" + q
		if timeout != "" {
			path += "?timeout=" + timeout
		}
		return httptest.NewRequest(http.MethodGet, path, nil)
	}
	reqPush := func(q, val string) *http.Request {
		path := "/" + q
		if val != "" {
			path += "?v=" + val
		}
		return httptest.NewRequest(http.MethodPut, path, nil)
	}

	// push responds with bad request if no value provided
	{
		recorder := httptest.NewRecorder()
		errPrefix := "push with no value"

		mux.ServeHTTP(recorder, reqPush("any", ""))

		assertStatusCode(t, recorder, http.StatusBadRequest, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)
	}

	// pop from non-existent queue responds with not found and empty body
	{
		recorder := httptest.NewRecorder()
		errPrefix := "pop from non-existent queue"

		mux.ServeHTTP(recorder, reqPop("non-existent", ""))

		assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)
	}

	// sequence of pushes and pops
	{
		errPrefix := "sequence of pushes and pops"

		petQueueName := "pet"
		petQueueVal1 := "cat"
		petQueueVal2 := "dog"

		roleQueueName := "role"
		roleQueueVal1 := "manager"
		roleQueueVal2 := "executive"

		// pushes
		recorder := httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPush(petQueueName, petQueueVal1))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPush(petQueueName, petQueueVal2))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPush(roleQueueName, roleQueueVal1))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPush(roleQueueName, roleQueueVal2))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		// pops
		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(petQueueName, ""))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertBody(t, recorder, petQueueVal1, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(petQueueName, ""))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertBody(t, recorder, petQueueVal2, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(petQueueName, ""))

		assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(petQueueName, ""))

		assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(roleQueueName, ""))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertBody(t, recorder, roleQueueVal1, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(roleQueueName, ""))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertBody(t, recorder, roleQueueVal2, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(roleQueueName, ""))

		assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)
	}

	// pop with timeout from queue with value
	{
		errPrefix := "pop with timeout from queue with value"
		petQueueName := "pet"
		petQueueVal := "cat"

		recorder := httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPush(petQueueName, petQueueVal))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertEmptyBody(t, recorder, errPrefix)

		recorder = httptest.NewRecorder()

		mux.ServeHTTP(recorder, reqPop(petQueueName, "2"))

		assertStatusCode(t, recorder, http.StatusOK, errPrefix)
		assertBody(t, recorder, petQueueVal, errPrefix)
	}

	// pop with timeout from empty queue
	{
		errPrefix := "pop with timeout from empty queue"
		petQueueName := "pet"

		recorder := httptest.NewRecorder()

		doneChannel := make(chan struct{}, 1)
		go func() {
			mux.ServeHTTP(recorder, reqPop(petQueueName, "2"))

			doneChannel <- struct{}{}
		}()

		select {
		case <-doneChannel:
			assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
			assertEmptyBody(t, recorder, errPrefix)
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: no response was received after 3 second awaiting", errPrefix)
		}
	}

	// pop with timeout from empty queue, but in the middle of waiting time value is pushed
	{
		errPrefix := "pop with timeout from empty queue, but in the middle of waiting time value is pushed"
		petQueueName := "pet"
		petQueueVal := "cat"

		popRecorder := httptest.NewRecorder()
		pushRecorder := httptest.NewRecorder()

		doneChannel := make(chan struct{}, 1)
		go func() {
			mux.ServeHTTP(popRecorder, reqPop(petQueueName, "2"))

			doneChannel <- struct{}{}
		}()

		go func() {
			time.Sleep(1 * time.Second)

			mux.ServeHTTP(pushRecorder, reqPush(petQueueName, petQueueVal))
			assertStatusCode(t, pushRecorder, http.StatusOK, errPrefix)
			assertEmptyBody(t, pushRecorder, errPrefix)
		}()

		select {
		case <-doneChannel:
			assertStatusCode(t, popRecorder, http.StatusOK, errPrefix)
			assertBody(t, popRecorder, petQueueVal, errPrefix)
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: no response was received after 3 second awaiting", errPrefix)
		}
	}

	// queue clients with timeout for one queue
	{
		errPrefix := "queue clients with timeout for one queue"
		queue := "queue"

		for i := range 30 {
			time.Sleep(1 * time.Millisecond)
			go func() {
				recorder := httptest.NewRecorder()
				var timeout string
				var check func()
				if i%2 == 0 {
					timeout = "1"
					check = func() {
						assertStatusCode(t, recorder, http.StatusNotFound, errPrefix)
						assertEmptyBody(t, recorder, errPrefix)
					}
				} else {
					timeout = "5"
					check = func() {
						assertStatusCode(t, recorder, http.StatusOK, errPrefix)
						assertBody(t, recorder, strconv.Itoa(i/2), errPrefix)
					}
				}

				mux.ServeHTTP(recorder, reqPop(queue, timeout))
				check()
			}()
		}

		time.Sleep(1 * time.Second)
		for i := range 15 {
			mux.ServeHTTP(httptest.NewRecorder(), reqPush(queue, strconv.Itoa(i)))

		}
	}
}

func assertStatusCode(t *testing.T, recorder *httptest.ResponseRecorder, expectedStatusCode int, errPrefix string) {
	t.Helper()

	if recorder.Result().StatusCode != expectedStatusCode {
		t.Fatalf(
			"%s: expected to respond with %d but received %d",
			errPrefix, expectedStatusCode, recorder.Result().StatusCode,
		)
	}
}

func assertBody(t *testing.T, recorder *httptest.ResponseRecorder, expectedBody, errPrefix string) {
	t.Helper()

	bodyBytes, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatalf("%s: reading body err: %s", errPrefix, err.Error())
	}

	receivedBody := string(bodyBytes)

	if receivedBody != expectedBody {
		t.Fatalf("%s: expected body: %q, but received: %q", errPrefix, expectedBody, receivedBody)
	}
}

func assertEmptyBody(t *testing.T, recorder *httptest.ResponseRecorder, errPrefix string) {
	t.Helper()

	assertBody(t, recorder, "", errPrefix)
}

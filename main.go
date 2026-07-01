package main

import (
	"log"
	"net/http"
	"os"
)

type (
	queueStorage map[string]*queue

	queue struct {
		head *queueElement
		tail *queueElement
	}

	queueElement struct {
		value string
		next  *queueElement
	}
)

var qStor = make(queueStorage)

func (qs queueStorage) Pop(queueName string) *string {
	q, exists := qs[queueName]
	if !exists || q.head == nil {
		return nil
	}

	el := q.head
	q.head = el.next
	if q.head == nil {
		q.tail = nil
	}

	return &el.value
}

func (qs queueStorage) Push(queueName, value string) {
	qElement := &queueElement{
		value: value,
	}

	q, exists := qStor[queueName]
	if !exists || q.tail == nil {
		q = &queue{
			head: qElement,
			tail: qElement,
		}

		qStor[queueName] = q
	} else {
		q.tail.next = qElement
		q.tail = q.tail.next
	}
}

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatal("missing required command line argument \"port\"")
	}

	port := args[1]

	mux := http.NewServeMux()

	mux.HandleFunc(`PUT /{queue}`, PushMessage)
	mux.HandleFunc(`GET /{queue}`, PopMessage)

	addr := ":" + port
	log.Printf("listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func PushMessage(w http.ResponseWriter, r *http.Request) {
	value := r.URL.Query().Get("v")
	if value == "" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	qName := r.PathValue("queue")

	qStor.Push(qName, value)

	w.WriteHeader(http.StatusOK)
}

func PopMessage(w http.ResponseWriter, r *http.Request) {
	timeout := r.URL.Query().Get("timeout")
	if timeout != "" {
		w.WriteHeader(http.StatusNotImplemented)

		return
	}

	qName := r.PathValue("queue")

	value := qStor.Pop(qName)
	if value == nil {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	w.Write([]byte(*value))
	w.WriteHeader(http.StatusOK)
}

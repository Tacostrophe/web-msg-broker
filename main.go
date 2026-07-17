package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var queueSvc = &QueueService{storage: make(map[string]*queueOfElements)}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("port arg is required")
	}
	addr := ":" + os.Args[1]

	http.HandleFunc("GET /{queue_name}", popMessage)
	http.HandleFunc("PUT /{queue_name}", pushMessage)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server listening and serving error: %s", err.Error())
	}
}

func popMessage(w http.ResponseWriter, r *http.Request) {
	queueName := r.PathValue("queue_name")
	value := queueSvc.Pop(queueName)
	if value != "" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(value))
		return
	}

	if timeoutStr := r.URL.Query().Get("timeout"); timeoutStr != "" {
		timeout, err := strconv.Atoi(timeoutStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
		defer cancel()

		if value = <-queueSvc.Subscribe(ctx, queueName); value != "" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(value))
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func pushMessage(w http.ResponseWriter, r *http.Request) {
	queueName := r.PathValue("queue_name")
	value := r.URL.Query().Get("v")
	if value == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queueSvc.Push(queueName, value)
	w.WriteHeader(http.StatusOK)
}

type QueueService struct {
	mutex   sync.Mutex
	storage map[string]*queueOfElements
}

func (qs *QueueService) Pop(queueName string) string {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	queue, exists := qs.storage[queueName]
	if !exists {
		return ""
	}

	return queue.Pop()
}

func (qs *QueueService) Push(queueName, value string) {
	qs.get(queueName).Push(value)
}

func (qs *QueueService) Subscribe(ctx context.Context, queueName string) <-chan string {
	queue := qs.get(queueName)
	resChannel := make(chan string, 1)
	go queue.Subscribe(ctx, resChannel)

	return resChannel
}

func (qs *QueueService) get(queueName string) *queueOfElements {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	q, exists := qs.storage[queueName]
	if !exists {
		q = &queueOfElements{queue: queue[string]{}}
		qs.storage[queueName] = q
	}

	return q
}

type queueOfElements struct {
	queue[string]
	Subs queue[chan string]
}

func (q *queueOfElements) Subscribe(ctx context.Context, resChannel chan string) {
	removeChan := q.Subs.Push(resChannel)

	<-ctx.Done()
	removeChan()
	close(resChannel)
}

func (q *queueOfElements) Push(value string) func() {
	if resChan := q.Subs.Pop(); resChan != nil {
		resChan <- value
		return func() {}
	}

	return q.queue.Push(value)
}

type (
	queue[T any] struct {
		mutex sync.Mutex
		head  *queueNode[T]
		tail  *queueNode[T]
	}

	queueNode[T any] struct {
		next  *queueNode[T]
		value T
	}
)

func (q *queue[T]) Pop() T {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.head == nil {
		var empty T
		return empty
	}

	value := q.head.value
	q.head = q.head.next

	if q.head == nil {
		q.tail = nil
	}

	return value
}

func (q *queue[T]) Push(value T) func() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	newNode := &queueNode[T]{value: value}

	var previous *queueNode[T]
	if q.tail == nil {
		q.head, q.tail = newNode, newNode
	} else {
		q.tail.next = newNode
		previous = q.tail
		q.tail = q.tail.next
	}

	return func() {
		q.mutex.Lock()
		defer q.mutex.Unlock()

		if previous != nil {
			previous.next = newNode.next
		}

		if q.head == newNode {
			q.head, q.tail = nil, nil
		} else if q.tail == newNode {
			q.tail = previous
		}
	}
}

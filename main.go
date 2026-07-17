package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	qStor = NewQueueStorage() // consider mv to main

	errTimeout = errors.New("timed out")
)

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

	q, exists := qStor.Get(qName)
	if !exists {
		q = NewQueue(value)
		qStor.Add(qName, q)
	} else {
		q.Push(value)
	}

	w.WriteHeader(http.StatusOK)
}

func PopMessage(w http.ResponseWriter, r *http.Request) {
	qName := r.PathValue("queue")

	val := popMessageSync(qName)
	if val != nil {
		w.Write(val)
		w.WriteHeader(http.StatusOK)

		return
	}

	timeoutStr := r.URL.Query().Get("timeout")
	if timeoutStr != "" && timeoutStr != "0" {
		timeout, err := strconv.Atoi(timeoutStr)
		if err != nil || timeout < 0 {
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
		defer cancel()

		res, ok := <-qStor.Subscribe(ctx, qName)
		if ok {
			w.Write([]byte(res))
			w.WriteHeader(http.StatusOK)

			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func popMessageSync(qName string) []byte {
	q, exists := qStor.Get(qName)
	if !exists {
		return nil
	}

	value := q.Pop()
	if value == nil {
		return nil
	}

	return []byte(*value)
}

type QueueStorage struct {
	mutex   sync.RWMutex
	storage map[string]*Queue
}

func NewQueueStorage() *QueueStorage {
	storage := make(map[string]*Queue)

	return &QueueStorage{storage: storage}
}

func (qs *QueueStorage) Get(qName string) (*Queue, bool) {
	qs.mutex.RLock()
	defer qs.mutex.RUnlock()

	q, exists := qs.storage[qName]
	return q, exists
}

func (qs *QueueStorage) Add(qName string, q *Queue) {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	qs.storage[qName] = q
}

func (qs *QueueStorage) Subscribe(ctx context.Context, qName string) <-chan string {
	q, exists := qs.Get(qName)
	if !exists {
		q = NewQueue("")
		qs.Add(qName, q)
	}

	resChan := make(chan string, 1)

	q.Subscribe(ctx, resChan)

	return resChan
}

type (
	Queue struct {
		mutex sync.Mutex
		head  *queueElement
		tail  *queueElement

		subs *SubsQueue
	}

	queueElement struct {
		value string
		next  *queueElement
	}
)

func NewQueue(value string) *Queue {
	var qElement *queueElement
	if value != "" {
		qElement = &queueElement{
			value: value,
		}
	}

	return &Queue{
		head: qElement,
		tail: qElement,

		subs: &SubsQueue{},
	}
}

func (q *Queue) Pop() *string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.head == nil {
		return nil
	}

	el := q.head
	q.head = el.next

	if q.head == nil {
		q.tail = nil
	}

	return &el.value
}

func (q *Queue) Push(value string) {
	if resChan := q.subs.Pop(); resChan != nil {
		resChan <- value

		return
	}

	qElement := &queueElement{
		value: value,
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.tail == nil {
		q.head = qElement
		q.tail = qElement
	} else {
		q.tail.next = qElement
		q.tail = q.tail.next
	}
}

func (q *Queue) Subscribe(ctx context.Context, resChan chan string) {
	close := q.subs.Push(resChan)
	defer close()

	<-ctx.Done()
}

type (
	SubsQueue struct {
		mutex sync.Mutex
		head  *sub
		tail  *sub
		len   int
	}

	sub struct {
		res      chan string
		next     *sub
		previous *sub
		mutex    sync.Mutex
	}
)

func (q *SubsQueue) Pop() chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.len == 0 {
		return nil
	}

	sub := q.head
	q.head = sub.next

	if q.head == nil {
		q.tail = nil
	} else {
		q.head.previous = nil
	}

	q.len--

	return sub.res
}

func (q *SubsQueue) Push(res chan string) func() {
	q.mutex.Lock()

	sub := &sub{
		res:      res,
		previous: q.tail,
	}

	if q.tail == nil {
		q.head = sub
		q.tail = sub
	} else {
		q.tail.next = sub
		q.tail = q.tail.next
	}

	q.len++

	q.mutex.Unlock()

	return func() {
		q.mutex.Lock()
		defer q.mutex.Unlock()

		if sub.previous == nil {
			q.head = sub.next
		} else {
			sub.previous.next = sub.next
		}

		if sub.next != nil {
			sub.next.previous = sub.previous
		} else {
			q.tail = sub.previous
		}

		q.len--

		close(res)
	}
}

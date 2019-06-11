package taskchain

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestTask(t *testing.T) {
	runtime.GOMAXPROCS(2)
	p := New(createOp(2))

	c1 := New(createOp(3))
	c2 := New(createOp(5))
	cc1 := New(createOp(7))

	p.AddChildren(c1, c2)
	c1.AddChildren(cc1)

	input, output, err := p.Start()
	if err != nil {
		t.Errorf("task failed to run: %v", err)
	}

	go func() {
		input <- 1
		input <- 2
		close(input)
	}()

	var results []int
	for result := range output {
		results = append(results, result.(int))
	}
	if err := p.Err(); err != nil {
		t.Error(err)
	}
	sort.Slice(results, func(i, j int) bool { return results[i] > results[j] })
	expect := []int{84, 42, 20, 10}
	if !reflect.DeepEqual(expect, results) {
		t.Errorf("results: %v, expected: %v", results, expect)
	}
}

func TestTaskChildRunFail(t *testing.T) {
	p := New(createOp(2))
	c1 := New(createOp(3))
	p.AddChildren(c1)
	_, _, err := c1.Start()
	if err == nil {
		t.Error("err should not be nil")
	}
}

func TestTimeoutWithContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	sleep := createSleepOp(time.Second)
	p := New(sleep)
	c := New(sleep)
	cc := New(sleep)
	ccc := New(sleep)
	p.AddChildren(c)
	c.AddChildren(cc)
	cc.AddChildren(ccc)
	in, out, err := p.Start()
	if err != nil {
		t.Errorf("task failed to run: %v", err)
	}
	go func() {
		<-ctx.Done()
		err := p.Terminate()
		if err != nil {
			t.Error(err)
		}
	}()
	go func() {
		in <- 0
		close(in)
	}()

	if _, ok := <-out; ok {
		t.Error("Termination failed (receiver must be canceled)")
	}
}

func TestErrChan(t *testing.T) {
	p := New(createOp(2))
	c1 := New(createOp(3))
	c2 := New(createSleepOp(10 * time.Second))
	cc2 := New(createOp(5))
	cc1 := New(createErrOp())

	p.AddChildren(c1, c2)
	c1.AddChildren(cc1)
	c2.AddChildren(cc2)

	input, output, err := p.Start()
	if err != nil {
		t.Errorf("task failed to run: %v", err)
	}

	go func() {
		input <- 1
		input <- 2
		close(input)
	}()

	for range output {
	}

	if err := p.Err(); err == nil {
		t.Error("err should not be nil")
	}
}

func TestTerminatedTaskFail(t *testing.T) {
	p := New(createOp(2))
	if err := p.Terminate(); err == nil {
		t.Errorf("err should not be nil")
	}
}

func createOp(m int) func(in interface{}) (interface{}, error) {
	return func(in interface{}) (interface{}, error) {
		return in.(int) * m, nil
	}
}

func createSleepOp(d time.Duration) func(in interface{}) (interface{}, error) {
	return func(in interface{}) (interface{}, error) {
		time.Sleep(d)
		return in, nil
	}
}

func createErrOp() func(in interface{}) (interface{}, error) {
	return func(in interface{}) (interface{}, error) {
		return nil, errors.New("error in op")
	}
}

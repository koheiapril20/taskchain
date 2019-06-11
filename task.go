package taskchain

import (
	"errors"
	"sync"
)

// Task is a tree-structured data proccessing task which propagates output data to all child nodes.
type Task struct {
	root    bool
	head    *chan interface{}
	tail    *chan interface{}
	errChan *chan error
	// used for stopping execution. only root's done channel is used
	done     *chan struct{}
	children *[]*Task
	op       func(done *<-chan struct{}, in *<-chan interface{}, out *chan<- interface{}, errChan *chan<- error)
}

func (f *Task) merge(done *<-chan struct{}) *chan interface{} {
	if f.children == nil || len(*f.children) == 0 {
		return f.tail
	}
	tails := make([]chan interface{}, len(*f.children))
	for i, child := range *f.children {
		tails[i] = *child.merge(done)
	}
	merged := merge(done, tails...)
	return &merged
}

func (f *Task) runOp(in *<-chan interface{}, out *chan<- interface{}) {
	var errChan chan<- error = *f.errChan
	go func(done *chan struct{}, in *<-chan interface{}, out *chan<- interface{}, errChan *chan<- error) {
		defer close(*out)

		// check if done channel is still open
		ok := true
		select {
		case _, ok = <-*done:
		default:
		}

		if ok {
			var doneChan <-chan struct{} = *done
			f.op(&doneChan, in, out, errChan)
		}
	}(f.done, in, out, &errChan)
}

// Start makes the task ready to process data and returns i/o and error channels.
func (f *Task) Start() (chan<- interface{}, <-chan interface{}, <-chan error, error) {
	select {
	case _, ok := <-*f.done:
		if !ok {
			return nil, nil, nil, errors.New("task already terminated")
		}
	default:
	}
	ec := make(chan error)
	f.errChan = &ec
	if !f.root {
		go func(ec chan error) {
			ec <- errors.New("child task node cannot execute Run()")
			close(ec)
		}(ec)
	} else {
		f.run()
	}

	// error channel
	var errChan <-chan error = *f.errChan

	// input channel
	var input chan<- interface{} = *f.head

	// output channel
	var output <-chan interface{}
	var done <-chan struct{} = *f.done
	output = *f.merge(&done)

	return input, output, errChan, nil
}

func (f *Task) run() {
	// receive from head, execute operation and send to tail
	var input <-chan interface{} = *f.head
	var output chan<- interface{} = *f.tail
	f.runOp(&input, &output)

	if f.children != nil && len(*f.children) > 0 {
		// call run recursively for each child (child head to child tail)
		for i := range *f.children {
			func(c *Task) {
				c.errChan = f.errChan
				c.done = f.done
				c.run()
			}((*f.children)[i])
		}
		// make data pass through from tail to child head for each child
		go func(f *Task) {
			for _, child := range *f.children {
				defer close(*child.head)
			}
			var wg sync.WaitGroup
			for v := range *f.tail {
				for i := range *f.children {
					wg.Add(1)
					go func(v interface{}, child *Task) {
						defer wg.Done()
						select {
						case *child.head <- v:
						case <-*f.done:
							return
						}
					}(v, (*f.children)[i])
				}
			}
			wg.Wait()
		}(f)
	}
}

// AddChildren provides child nodes addition.
func (f *Task) AddChildren(c ...*Task) {
	for i := range c {
		c[i].root = false
	}
	*f.children = append(*f.children, c...)
}

// GetChildren returns child tasks.
func (f *Task) GetChildren() *[]*Task {
	return f.children
}

// Terminate terminates task.
func (f *Task) Terminate() error {
	if !f.root {
		return errors.New("child task cannot use Terminate")
	}
	close(*f.done)
	return nil
}

// Done returns done channel of this task.
func (f *Task) Done() <-chan struct{} {
	var done <-chan struct{} = *f.done
	return done
}

// New creates a new task with specified operation.
func New(op func(interface{}) (interface{}, error)) *Task {
	head := make(chan interface{})
	tail := make(chan interface{})
	done := make(chan struct{})
	f := &Task{
		root:     true,
		head:     &head,
		tail:     &tail,
		done:     &done,
		children: &[]*Task{},
	}
	f.setOp(op)
	return f
}

func (f *Task) setOp(op func(in interface{}) (interface{}, error)) {
	f.op = func(done *<-chan struct{}, in *<-chan interface{}, out *chan<- interface{}, errChan *chan<- error) {
		for v := range *in {
			select {
			case <-*done:
				return
			default:
			}
			result, err := op(v)
			if err != nil {
				*errChan <- err
				return
			}
			*out <- result
		}
	}
}

func merge(done *<-chan struct{}, cs ...chan interface{}) chan interface{} {
	var wg sync.WaitGroup
	out := make(chan interface{})

	// Start an output goroutine for each input channel in cs.  output
	// copies values from c to out until c is closed, then calls wg.Done.
	output := func(c chan interface{}) {
		defer wg.Done()
		for n := range c {
			select {
			case out <- n:
			case <-*done:
				// invoked by closing done channel
				return
			}

		}
	}
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		defer close(out)
		wg.Wait()
	}()
	return out
}

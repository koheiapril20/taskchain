# taskchain

a async tree-structured task chain runner for golang

```
func sample() {
	// sample operation creator
	multipleOp := func(in interface{}) (out interface{}, err error) {
		return func(in interface{}) (out interface{}, err error) {
			out = in.(int) * m
			return
		}
	}

	// create task tree
	p := taskchain.New(multipleOp(2))
	c1 := taskchain.New(multipleOp(3))
	c2 := taskchain.New(multipleOp(5))
	cc1 := taskchain.New(multipleOp(7))
	p.AddChildren(c1, c2)
	c1.AddChildren(cc1)

	input, output, err := p.Start()
	if err != nil {
		// failed to start
	}

	go func() {
		input <- 1
		input <- 2
		close(input) // tells the end of inputs
	}()

	// collects outputs
	// results: []int{84, 42, 20, 10} (order depends on execution)
	var results []int
	for result := range output {
		results = append(results, result.(int))
	}

	if err := p.Err(); err != nil {
		// error during operations
	}
}
```

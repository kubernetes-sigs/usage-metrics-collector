package testutil_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/usage-metrics-collector/pkg/testutil"
	"sigs.k8s.io/yaml"
)

// Client type which exposes different complexities for computing
// fibonacci numbers
type Client struct {
	Complexity string `json:"complexity"`
}

// FibonacciRunner runs a specified fibonacci methods on an array
// of numbers
func (c *Client) FibonacciRunner(inputs []int) []int64 {
	series := make([]int64, 0, len(inputs))

	for _, val := range inputs {
		fib := int64(0)

		switch c.Complexity {
		case "exponential":
			fib = c.FibonacciExponential(val)
		case "linear":
			fib = c.FibonacciLinear(val)
		default:
			fib = c.FibonacciLogarithmic(val)
		}

		series = append(series, fib)
	}
	return series
}

// FibonacciExpenenetial computes the nth fibonacci number with
// with exponential complexity
func (c *Client) FibonacciExponential(n int) int64 {
	if n == 0 {
		return 0
	}

	if n == 1 || n == 2 {
		return 1
	}
	return c.FibonacciExponential(n-1) + c.FibonacciExponential(n-2)
}

// FibonacciLinear computes the nth fibonacci number in linear time
func (c *Client) FibonacciLinear(n int) int64 {
	if n == 0 {
		return 0
	}

	a := int64(1)
	b := int64(1)

	for i := n; i > 1; i-- {
		at := a
		a = b
		b = at + b
	}
	return a
}

// FibonacciLogarithmic computes the nth fibonacci number in log time
func (c *Client) FibonacciLogarithmic(n int) int64 {
	if n == 0 {
		return 0
	}

	a := int64(1)
	b := int64(1)
	p := int64(0)
	q := int64(1)

	for i := n; i > 2; {
		if i%2 == 0 {
			pt := p
			p = p*p + q*q
			q = q*q + 2*q*pt
			i /= 2
		} else {
			i--
		}
		at := a
		a = p*a + q*b
		b = q*at + b*(p+q)
	}
	return b
}

// stringToInts converts a string slice to an int slice
func stringsToInts(sa []string) ([]int, error) {
	var values = make([]int, 0, len(sa))
	for _, s := range sa {
		val, err := strconv.Atoi(s)
		if err != nil {
			return values, err
		}

		values = append(values, val)
	}
	return values, nil
}

// int64sToString converts an int64 slice to a string
// with each int64 on a newline
func int64sToString(values []int64) string {
	output := ""
	l := len(values)
	for i, val := range values {
		s := strconv.FormatInt(val, 10)
		if i < l-1 {
			output += s + "\n"
		} else {
			output += s
		}
	}
	return output
}

func ExampleClient_FibonacciExponential() {
	input := map[string]string{
		"inputs.txt": "5,11,2,25,13,17,12,29,14,30",
		"input.yaml": "complexity: exponential",
	}

	testCase := testutil.TestCase{
		Name:   "FibonacciExponential",
		Inputs: input,
	}

	err := func(tc *testutil.TestCase) error {
		var c Client
		err := yaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), &c)
		if err != nil {
			return err
		}
		inputNumbers, _ := stringsToInts(strings.Split(tc.Inputs["inputs.txt"], ","))
		fibNumbers := c.FibonacciRunner(inputNumbers)
		tc.Actual = int64sToString(fibNumbers)
		fmt.Println("Fibonacci Algorithm: " + c.Complexity)
		fmt.Println("Expected: " + strings.ReplaceAll(tc.Actual, "\n", ","))
		return nil
	}(&testCase)
	fmt.Println(err)
	// Output: Fibonacci Algorithm: exponential
	// Expected: 5,89,1,75025,233,1597,144,514229,377,832040
	// <nil>
}

func ExampleClient_FibonacciLinear() {
	input := map[string]string{
		"inputs.txt": "49,30,42,35,40,50,36,57",
		"input.yaml": "complexity: linear",
	}

	testCase := testutil.TestCase{
		Name:   "FibonacciLinear",
		Inputs: input,
	}

	err := func(tc *testutil.TestCase) error {
		var c Client
		err := yaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), &c)
		if err != nil {
			return err
		}
		inputNumbers, _ := stringsToInts(strings.Split(tc.Inputs["inputs.txt"], ","))
		fibNumbers := c.FibonacciRunner(inputNumbers)
		tc.Actual = int64sToString(fibNumbers)
		fmt.Println("Fibonacci Algorithm: " + c.Complexity)
		fmt.Println("Expected: " + strings.ReplaceAll(tc.Actual, "\n", ","))
		return nil
	}(&testCase)
	fmt.Println(err)
	// Output: Fibonacci Algorithm: linear
	// Expected: 7778742049,832040,267914296,9227465,102334155,12586269025,14930352,365435296162
	// <nil>
}

func ExampleClient_FibonacciLogarithmic() {
	input := map[string]string{
		"inputs.txt": "71,68,48,42,76,64,60,78,40,57",
		"input.yaml": "complexity: logarithmic",
	}

	testCase := testutil.TestCase{
		Name:   "FibonacciLogarithmic",
		Inputs: input,
	}

	err := func(tc *testutil.TestCase) error {
		var c Client
		err := yaml.UnmarshalStrict([]byte(tc.Inputs["input.yaml"]), &c)
		if err != nil {
			return err
		}
		inputNumbers, _ := stringsToInts(strings.Split(tc.Inputs["inputs.txt"], ","))
		fibNumbers := c.FibonacciRunner(inputNumbers)
		tc.Actual = int64sToString(fibNumbers)
		fmt.Println("Fibonacci Algorithm: " + c.Complexity)
		fmt.Println("Expected: " + strings.ReplaceAll(tc.Actual, "\n", ","))
		return nil
	}(&testCase)
	fmt.Println(err)
	// Output: Fibonacci Algorithm: logarithmic
	// Expected: 308061521170129,72723460248141,4807526976,267914296,3416454622906707,10610209857723,1548008755920,8944394323791464,102334155,365435296162
	// <nil>
}

func Test(t *testing.T) {
	tcp := testutil.TestCaseParser{
		ExpectedSuffix: ".txt",
	}

	tcp.TestDir(t,
		func(tc *testutil.TestCase) error {
			var c Client
			tc.UnmarshalInputsStrict(map[string]interface{}{
				"input.yaml": &c,
			})
			inputNumbers, err := stringsToInts(strings.Split(tc.Inputs["inputs.txt"], "\n"))
			require.NoError(t, err)
			fibNumbers := c.FibonacciRunner(inputNumbers)
			tc.Actual = int64sToString(fibNumbers)

			return nil
		})
}

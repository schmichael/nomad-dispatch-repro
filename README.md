# nomad dispatch job tester

Following dispatch jobs is harder than it ought to be. This is a test repro
based on real code to demonstrate how difficult it is to get right.

This code will register a parameterized job and then dispatch many instances of
it: attempting to follow each dispatch to its completion.

## prerequisites

1. A running Nomad cluster (can be `nomad agent -dev`)
2. `NOMAD_ADDR`, `NOMAD_TOKEN`, and any other env vars necessary to register
	 jobs set.
3. Go

## running

```
# If you have the source checked out:
$ go run .
2024/11/14 15:56:06 [ 0:0   ] ok
2024/11/14 15:56:06 [ 0:0   ] ok
2024/11/14 15:56:06 [ 0:0   ] ok
2024/11/14 15:56:06 [ 1:0   ] ok
2024/11/14 15:56:06 [ 1:0   ] expected "sleeper" task but none is found for dispatch ID sleeper/dispatch-1731628560-bbbe6f03
... snip ...
2024/11/14 15:57:12 [ 2:9   ] ok
2024/11/14 15:57:12 [ 2:9   ] ok
2024/11/14 15:57:12 500 done after 1m12.399s with 53 errors
```

For certain errors the allocation(s) in question will be written to the current
directory as json such as: `feb895ce-2aaa-8460-c754-cf05a8127370.alloc.json`

Use `-help` to see and adjust test parameters.

The test does *not* cleanup after itself, but only creates 1 job (and a number
of dead allocs).

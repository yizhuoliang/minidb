# MiniDB User Manual

## running a test case
Simply put a single test case in a txt file in the same directory of main.go, then run the program with
`$ go run main.go [filename.txt]`, then the results will be **appended** to the output.txt file.

Something to note:
- if output.txt does not exist, the program will create one
- if output.txt already exist, the program will only append, not overwrite
- the input.txt can have comments, but not random strings that are not database commands and not comments prefixed with "//"
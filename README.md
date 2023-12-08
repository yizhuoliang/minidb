# MiniDB User Manual

## Running a test case
The `minidb` file is the compiled binary of my project, and that is compiled and reproziped on cims.
Simply put a single test case in a txt file in the same directory of main.go, then run the program with
`./minidb [filename.txt]`, then the results will be **appended** to the output.txt file.
With reprozip, you can also run 
```
./reprounzip directory setup coulson-minidb.rpz ~/reprounzip_coulson-minidb
./reprounzip directory run ~/reprounzip_coulson-minid

```
Then you can switch to the reprounzip generated directory and check out the output.txt file there.

Something to note:
- if output.txt does not exist, the program will create one
- if output.txt already exist, the program will only append, not overwrite
- the input.txt can have comments, but not random strings that are not database commands and not comments prefixed with "//"
- to view the output.txt file comfortably, I suggest to make sure the terminal window is wide enough to list one line of dump information so that everything will be organized
- I would suggest removing the output.txt file before start so you can start testing with a clean file

## Rebuilding on your machine
As long as you have go installed on your machine, you can simply build it in one line
```
go build
```
and the output executable will be called `minidb`
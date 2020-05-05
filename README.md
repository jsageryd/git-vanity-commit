# git-vanity-commit

[![License MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/jsageryd/git-vanity-commit#license)

## Installation
```
go get -u github.com/jsageryd/git-vanity-commit
```

## Usage example
```
$ git init && git add . && git commit -m 'Initial commit'
Initialized empty Git repository in /Users/j/go/src/github.com/jsageryd/git-vanity-commit/.git/
[master (root-commit) 8f155be] Initial commit
 4 files changed, 237 insertions(+)
 create mode 100644 LICENSE
 create mode 100644 README.md
 create mode 100644 main.go
 create mode 100644 main_test.go
$ git-vanity-commit -prefix=c0ffee -reset
21:20:20 | Using commit at HEAD (8f155be96971)
21:20:20 | Finding hash prefixed "c0ffee"
21:20:31 | Found c0ffee9c331bc17c9e9605fcfbdcfcd439ada240 (10.786s)
21:20:31 | HEAD is now at c0ffee9c331bc17c9e9605fcfbdcfcd439ada240
$ git log --oneline
c0ffee9 (HEAD -> master) Initial commit
```

## License
Copyright (c) 2020 Johan Sageryd <j@1616.se>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

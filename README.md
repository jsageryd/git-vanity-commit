# git-vanity-commit

[![License MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/jsageryd/git-vanity-commit/blob/master/LICENSE)

## Installation
```
go install github.com/jsageryd/git-vanity-commit@latest
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
```

```
$ git-vanity-commit -prefix=c0ffee -reset
21:20:20 | Using commit at HEAD (8f155be96971)
21:20:20 | Finding hash prefixed "c0ffee"
21:20:31 | Found c0ffee9c331bc17c9e9605fcfbdcfcd439ada240 (10.786s)
21:20:31 | HEAD is now at c0ffee9c331bc17c9e9605fcfbdcfcd439ada240
```

```
$ git log --oneline
c0ffee9 (HEAD -> master) Initial commit
```

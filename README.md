# git-vanity-commit

[![License MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/jsageryd/git-vanity-commit/blob/master/LICENSE)

## Installation
```
go install github.com/jsageryd/git-vanity-commit@latest
```

## Usage example
```
$ git-vanity-commit -prefix=c0ffee -reset
17:03:16 | Using commit at HEAD (a27993c18f78)
17:03:16 | Finding hash prefixed "c0ffee"
17:03:16 | Commit size 154 bytes
17:03:16 | Using 8 concurrent workers
17:03:16 | Tested 39,051,709 commits at 80,349,673 commits per second
17:03:16 | Found c0ffee83124285d152bd620725476c8a0eb9714e (iteration 39051708, 486ms)
17:03:16 | Commit object written
17:03:16 | HEAD is now at c0ffee83124285d152bd620725476c8a0eb9714e
```

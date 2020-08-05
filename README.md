# Protocol Design: Robust File Transfer (RFT)

Protocol-Design project.

## Introduction

Directory structure:

| Directory       | Purpose       |
|-----------------|-----------------------------------------------------------------------------------------------|
| `cmd`           | Contains cli utilities to run a RFT server or client                                          |
| `rft`           | Contains a reference implementation for RFT written as golang library                         |


## Specification

The RFT-specification is written in a separate (currently closed source) repository.

## Implementation

This repository provides a reference implementation of RFC written in
[golang](https://golang.org/). To run a sample server and client to fetch
this File:

```shell
go build
./rft -s -t 9090 0.0.0.0 . &
./rft localhost -t 9090  README.md
```

For more options run `./rft -h`.

## Implementation test

There's an integration test runner, that executes an `rft` binary to start a
server and executes some test file transmissions. It can be used like this:

```shell
go build
./rft bench <rftbinary1> <rftbinary2>
```

With *rftbinary1* and *rftbinary2* being 2 implementations which will be tested
against each other.


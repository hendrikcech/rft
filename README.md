# protocol-design

Protocol-Design project.

## Introduction

Directory structure:

| Directory       | Purpose       |
|-----------------|-----------------------------------------------------------------------------------------------|
| `assignment1`   | Contains documentation and RFC Internet Draft for the RFT protocol                            |
| `cmd`           | Contains cli utilities to run a RFT server or client                                          |
| `docs`          | Contains meeting notes                                                                        |
| `misc`          | Contains useful content like reference programs that are not (yet) directly used in our work  |
| `rft`           | Contains a reference implementation for RFT written as golang library                         |


## Specification

The RFT-specification is written in the Internet-Draft style of the IETF RFCs.
The specification is written as Markdown in `assignment1/draft-rft.md` and then
compiled to XML using [mmark](https://github.com/mmarkdown/mmark/) and from xml to text or html using [xml2rfc](https://xml2rfc.tools.ietf.org/).

To compile the document, make sure to install these dependencies and then use
the `compile-md` script.

## Implementation

This repository provides a reference implementation of RFC written in
[golang](https://golang.org/). To run a sample server and client to fetch
this File:

```shell
go build
./rft -s -t 9090 &
./rft localhost -t 9090  README.md
```

For more options run `./rft -h`.


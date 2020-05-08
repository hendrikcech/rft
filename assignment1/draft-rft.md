%%%
title = "Robust File Transfer"
abbrev = "RFT"
ipr = "trust200902"
area = "Internet"
workgroup = "Network Working Group"
submissiontype = "IETF"
keyword = [""]
#date = 2020-04-29T00:00:00Z

[seriesInfo]
name = "RFC"
value = "2100"
stream = "IETF"
status = "informational"

[[author]]
initials="H."
surname="Cech"
fullname="Hendrik Cech"
organization = "TUM"
  [author.address]
  email = "hendrik.cech@tum.de"

[[author]]
initials="M."
surname="Engelbart"
fullname="Mathis Engelbart"
organization="TUM"
  [author.address]
  email="mathis.engelbart@tum.de"
%%%

{mainmatter}

# Introduction
This RFC describes the protocol "Robust File Transfer" (RFT) that defines the interaction between a server that offers files for clients to download. The communication employs a binary protocol and messages are transferred over the network using UDP.

The authors of this RFC follow the recommendations of RFC4101 "Writing Protocol Models".

# Requirements
- resumable transfers
- reliable transfers
    - checksum validation of transmitted files
- adjusts transfer speeds to avoid link congestion

# Protocol Overview
An RFT server listens for UDP packets at a well-known port. It is assumed that clients and servers can exchange packets using UDP.

## File Request
Transfers are initiated by clients who send a list of the desired files to the server. Each file is identified by their path on the server. Optionally, clients can request to not transfer the whole file but start the transfer from a specified byte offset.

The server replies with the total size and checksum of each file before starting with the data transfer.

TODO: how should the server indicate errors, i.e., that a file could not be found or is not readable?

## Data Transfer
## Transfer Termination

# Dealing with network issues
## Detecting and handling packet loss
## Resuming transfers

# Security Considerations
## DDOS
Server waits for an ACK of the file request response message before starting to send data packets.

# Message Formats
## Header

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|             Size              |           Version             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Message Type        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

All message types which are described in the following sections are prepended by
this header.

## File-Request
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|       Number of Files         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              |        Path Name (Length)    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              |        Path Name (Length)    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                            ...                                |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## Response to File Request
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           File Index          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Offset                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                              Size                             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                              Size                             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         CheckSum (64?)                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## Data
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         File Index                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           Offset                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           Offset                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Data (size from header)                    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## Acknowledgement
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         File Index                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of Last received byte                   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of Last received byte                   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Offset of missing byte                      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Offset of missing byte                      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             ...                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             ...                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## Close
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|            File Index         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```


<!-- # Credits -->

{backmatter}

<reference anchor='libes' target=''>
 <front>
 <title>Choosing a Name for Your Computer</title>
  <author initials='D.' surname='Libes' fullname='D. Libes'></author>
  <date year='1989' month='November'/>
 </front>
 <seriesInfo name="Communications of the ACM" value='Vol. 32, No. 11, Pg. 1289' />
</reference>

<reference anchor='lottor' target='namedroppers@internic.net'>
 <front>
 <title>Domain Name Survey</title>
  <author initials='M.' surname='Lottor' fullname='M. Lottor'></author>
  <date year='1997' month='January'/>
 </front>
</reference>

<reference anchor='wong' target='http://www.seas.upenn.edu/~mengwong/coolhosts.html'>
 <front>
 <title>Cool Hostnames</title>
  <author initials='M.' surname='Wong' fullname='M. Wong'></author>
  <date/>
 </front>
</reference>

<reference anchor='ts' target=''>
 <front>
 <title>Old Possum's Book of Practical Cats</title>
  <author initials='TS' surname='Stearns' fullname='TS. Stearns'></author>
  <date/>
 </front>
</reference>

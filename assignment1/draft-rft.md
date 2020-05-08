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

## Data Transfer
## Transfer Termination

# Flow and congestion control
The server only sends data that was requested by the client. The client only requests as much data as he can buffer. 

The amount of data this is in-flight is limited by the respective congestion window (*cwnd*).

## Flow Control
Client requests range of bytes from server.

Client MUST send at least one request every X time units.

### Default fixed buffer size algorithm
The client allocates a receive buffer of fixed size S. Initially he requests S much data.

Every time the client receives data, he inserts that data into the appropriate place in the receive buffer. Then, the client checks if a contineous block of data from the start of the requested file has been received. If so, that block of data is extracted from the buffer and written to persistent storage. The client requests as much new data as necessary from the server to reach S.

## Congestion Control
Both server and client maintain a packet number that is inserted into every packet that they send; it is each time increased by one. By looking for gaps in the packet numbers, the other party can detect packet losses (a timeout will need to be employed because packets might arrive out of order). If a packet loss is detected, the next message of the other party will inform the sender about the packet loss.

Both parties maintain a *congestion window* (*cwnd*) that is initially set to X. Each party increase their *cwnd* by some amount if they receive a packet and no packet loss occured. If packet loss occured, the *cwnd* is halfed.

# Dealing with network issues
## Detecting and handling packet loss
## Resuming transfers

# Security Considerations
## DDOS
Server waits for an ACK of the file request response message before starting to send data packets.

# Message Formats
## Header
All message types which are described in the following sections are prepended by this header.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|             Size              |x| Packet Number (31 bit)      |
|                               |^- Packet loss detected (most  |
|                               |   significant bit)            |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
| Message Type  |    Version    | Length of options             |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|  Option type  | Option length |   Option length-many byte    ...
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|  Option type  | Option length |   Option length-many byte    ...
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

## File-Request
Sent by the client.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|       Number of Files         |         reserved              |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                         Offset (64 bit)                       |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|           Length              |        Path Name (Length)    ...
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                         Offset (64 bit)                       |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|           Length              |        Path Name (Length)    ...
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                            ...                                |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

## Response to File Request
Sent by the server. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|     File Index                |          Error code           |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                        Offset (64 bit)                        |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                         Size (64 bit)                         |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                   CheckSum (256 bit = 32 byte)                |
+                                                               +
|                        ...                                    |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

### Error codes
- 0x00: all OK
- 0x01: file not found
- 0x02: temporarily not available
- 0x03: connection closed
- ...

## Data
Sent by the server.
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|           File Index          |       reserved                |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                      Offset (64 bit)                          |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                   Data (size from header)                    ...
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

## Acknowledgement
Sent by the client. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|         File Index            |        Number of gaps         |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|            Offset of last received byte (64 bit)              |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                Offset of first missing byte                   |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                Offset of last missing byte                    |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                             ...                               |
+                                                               +
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

## Request (Acknowledgement alternative)
Sent by the client. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|         File Index             |                              |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                Offset of first requested byte                 |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                Offset of last requested byte                  |
+                                                               +
|                                                               |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|                             ...                               |
+                                                               +
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
```

## Close
Sent by the client.
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
|            File Index         |              ...              |
+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+-|-+
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

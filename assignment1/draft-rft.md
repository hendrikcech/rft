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
value = "draft-01"
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

<!-- Guideline: RFC4101 "Writing Protocol Models" -->
# Introduction
This RFC describes the protocol "Robust File Transfer" (RFT) that defines the interaction between a server that offers files and clients that can download those files. The communication employs a binary protocol and messages are transferred over the network using UDP.
<!-- TODO: "reference" the UDP RFC? -->

RFT secures a successful transmission even in the face of network problems such as message loss and reordering. The protocol is able to efficiently resume aborted transfers and to verify the integrity of transferred files. Transmission is performed with consideration towards the resource constraints of sender, receiver and network links.
<!-- # Requirements
- resumable transfers
- reliable transfers
    - checksum validation of transmitted files
- adjusts transfer speeds to ...
    - adapt to the available resources of the participants
    - avoid link congestion -->

This RFC is structured as follows. Section 1 gives a high-level intuition over the protocol's workings. Section 2 ...

# Protocol Overview
An RFT server continuously listens for UDP packets at an address that is known to potential clients.

```
    Client                          Server
       +                              +
       |  Requests files              |
       | +--------------------------> |
       |                              |
       | Respond with information     |
       | about requested files        |
       | <--------------------------+ |
       |                              |
       |                              |
       |                              |
       | Request byte range of file   |
       | +--------------------------> |
       |                              |
       |                              |
       | Send part of requested       |
       | byte range.                  |
       | <--------------------------+ |
       |                              |
       | Send part of requested       |
       | byte range.                  |
       | <--------------------------+ |
       |                              |
       |                              |
       | Request byte range of file   |
       | +--------------------------> |
       |                              |
       |               .              |
       |               .              |
       |               .              |
       +                              +
```

## File Request
Transfers are initiated by clients who send a single message with a list of desired files to the server. Each file is identified by their path on the server. The position of each file in the request list determines the handle by which  the file is referred to in the subsequent data request messages. For example: The file that is first in the request list is assigned index 0, the next file index 1, and so on. 

The server replies with a single message that contains the total size and checksum of each file that can be served. If a file can not be served, an explanatory error code is returned for that file.
<!-- Optionally, clients can request to not transfer the whole file but start the transfer from a specified byte offset. -->

## Data Transfer
The client picks at least one of the desired files for which no error was returned. It sends a data request message that contains the corresponding index of that file and a byte range. The first message typically starts requesting from byte 0.

The server receives the request and adds it to the request queue for that particular client. It starts fulfilling the request by sending parts of the requested byte range.

The client may not to renew its request before it has been fully served  as described in the upcoming flow and congestion control section. It may adapt the size of the requested byte range and the number of concurrently requested files as discussed in the section about performance considerations.

## Transfer Termination
Once the client has received all desired files, it may terminate the transmission program. The server may clean up any state after a timeout.

# Flow and congestion control

A server needs to limit the amount of data it sends in order to prevent overstressing the network or the receiving client. RFT uses the Flow Control mechanism described in [section 4.1](#FlowControl) to prevent overloading the receiver and the Congestion Control mechanism described in [section 4.2](#CongestionControl) to prevent overloading the available network resources.


## Flow Control {#FlowControl}

As described in the protocol overview, the client periodically requests a range of data from a server. The size of the requested data range MUST never be larger than the amount of data the client is currently able to receive. A server MUST not send more data than a client requested. It is possible, that a server sends less than the requested amount of data, when the requested file is smaller than the requested amount of data or the server is otherwise limited e.g. by congestion control. If, after sending all requested bytes, the server does not receive another data request from the client, even though the file has more bytes than requested, the server may close the connection after waiting for a period longer than a specified timeout. 

TODO: Specify timeout.

### Default fixed buffer size algorithm
TODO: Decide whether we need this section. Maybe it's to much implementation detail?

The client allocates a receive buffer of fixed size S. Initially he requests S much data.

Every time the client receives data, he inserts that data into the appropriate place in the receive buffer. Then, the client checks if a contineous block of data from the start of the requested file has been received. If so, that block of data is extracted from the buffer and written to persistent storage. The client requests as much new data as necessary from the server to have again requested S byte in total.

If the client suspects that packets have been lost, he should request those specific gap byte ranges again.

## Congestion Control

To avoid overstressing the network that is used for data transfer, RFT uses a congestion control mechanism, that keeps the data sending server from sending to much data at the same time. The receiving client indicates packet loss to the server as a sign to reduce the amount of data sent on the same time.
As long as no packet loss occurs, the server might slowly increase the amount it sends out on the network.

### Detecting Packet Loss

Each packet that a server sends is assigned to an instance of a congestion window. The server announces the size of the current congestion window in the *Size of CWND* field of the packet header. Additionally, it announces the number of packets, that can follow in the same congestion window, before a new congestion window needs to be used, in the *Number of free CWND slots* field of the packet header. A server MUST fill a congestion window, before opening a new one. A client MUST send a new data request, before a new congestion window can be opened. The client can infer whether some data packets were lost, by comparing the number of packets received for the current congestion window. If a packet is lost, the client MUST set the *packet loss bit* of the next data request to 1 and MAY re-request the missing data in a new data request. To avoid re-requesting data that is received by the client out of order, the client SHOULD wait a reasonable period of time before sending the new request.

### Handling Packet Loss
If the server receives a data request with the *packet loss bit* set to *0*, it SHOULD increase the size of the next congestion window. If the server receives a data request with the *packet loss bit* set to *1*, it SHOULD decrease the size of the next congestion window.
If the server does not receive a new data request after completely filling up a congestion window, it MAY close the connection after waiting a period of time.

# Dealing with network issues

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
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Size (16 bit)                |   Number of free CWND slots   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Size of CWND (16 bit)        |*| Msg Type    | # of options  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Option type  | Option length |   Option length-many byte    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Option type  | Option length |   Option length-many byte    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

*: Packet loss detected (if most significant bit = 1)
```

## File-Request
Sent by the client.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|       Number of Files         |         reserved              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Offset (64 bit)                       |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              |        Path Name (Length)    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Offset (64 bit)                       |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              |        Path Name (Length)    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                            ...                                |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

## Response to File Request
Sent by the server. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|     File Index                |          Error code           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Offset (64 bit)                        |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Size (64 bit)                         |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Checksum (256 bit = 32 byte)                |
+                                                               +
|                        ...                                    |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### Error codes
- 0x00: all OK
- 0x01: file not found
- 0x02: temporarily not available
- 0x03: connection closed
- ...


## Request (Acknowledgement alternative)
Sent by the client. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         File Index             |                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of first requested byte (64 bit)        |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of last requested byte (64 bit)         |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             ...                               |
+                                                               +
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```


## Data
Sent by the server.
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           File Index          |       reserved                |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                      Offset (64 bit)                          |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Data (size from header)                    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
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

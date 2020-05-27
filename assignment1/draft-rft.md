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

This RFC is structured as follows. (#ProtocolFlow) gives a high-level intuition over the protocol's workings. (#FlowCongestion) explains the flow and congestion control mechanisms used by RFT. The message types used are defined in (#MessageFormats).

# Protocol flow {#ProtocolFlow}
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

## File request
Transfers are initiated by clients who send a single message with a list of desired files to the server. Each file is identified by their path on the server. The position of each file in the request list determines the handle by which  the file is referred to in the subsequent data request messages. For example: The file that is first in the request list is assigned index 0, the next file index 1, and so on. 

The server replies with a single message that contains the total size and checksum of each file that can be served. If a file can not be served, an explanatory error code is returned for that file.
<!-- Optionally, clients can request to not transfer the whole file but start the transfer from a specified byte offset. -->

## Data transfer
The client picks at least one of the desired files for which no error was returned. It sends a data request message that contains the corresponding index of that file and a byte range. The first message typically starts requesting from byte 0.

The server receives the request and adds it to the request queue for that particular client. It starts fulfilling the request by sending parts of the requested byte range.

The client may not to renew its request before it has been fully served  as described in the upcoming flow and congestion control section. It may adapt the size of the requested byte range and the number of concurrently requested files as discussed in the section about performance considerations.

## Transfer termination
Once the client has received a file in total, it MAY compute the checksum and compare it with the one given by the server at connection start (see (#Checksum)).

Once the client has received all desired files, it may terminate the transmission program. The server may clean up any state after a timeout.

## Resuming transfers
A file transmission may have been interrupted which results in that file being only partially written to disk. An RFT client may resume the transmission of that file by starting to request data not from byte zero but from an offset.

# Flow and congestion control {#FlowCongestion}
A server needs to limit the amount of data it sends in order to prevent overstressing the network or the receiving client. RFT uses the flow control mechanism described in (#FlowControl) to prevent overloading the receiver and the congestion control mechanism described in (#CongestionControl) to prevent overloading the available network resources.


## Flow Control {#FlowControl}
As described in the protocol overview, a client periodically requests a range of data from a server. The size of the requested data range MUST never be larger than the amount of data the client is currently able to receive. A server MUST not send more data than a client requested. It is possible, that a server sends less than the requested amount of data, when the requested file is smaller than the requested amount of data or the server is otherwise limited, e.g., by congestion control. If, after sending all requested bytes, the server does not receive another data request from the client, even if the file has more bytes than requested, the server may close the connection after waiting for a period longer than a specified timeout (see (#Timeouts)). 

## Congestion Control {#CongestionControl}
To avoid overstressing the network that is used for data transfer, RFT uses a congestion control mechanism, that prevents the server from sending an overwhelming amount of data. The receiving client informs the server about packet loss which acts as a sign to reduce the data rate.
As long as no packet loss occurs, the server SHOULD slowly increase the sending rate.

### Detecting Packet Loss
Each packet that a server sends is assigned to an instance of a congestion window. The server announces the size of the current congestion window in the *CWND size* field of the packet header. Additionally, it announces the number of packets that can follow in the same congestion window, before a new congestion window needs to be used, in the *free CWND slots* field of the packet header. A server MUST fill a congestion window, before opening a new one. A client MUST send a new data request, before a new congestion window can be opened. The client can infer whether some data packets were lost, by comparing the number of packets received for the current congestion window with the announced congestion window size. To avoid re-requesting data that is received by the client out of order, the client SHOULD wait a reasonable period of time before sending the new request. The length of this time period SHOULD be a multiple of the average time measured between receiving two data packets. If a packet is lost, the client MUST set the *packet loss bit* of the next data request to 1 and MUST re-request the missing data in a new data request, unless it decides to terminate the connection without receiving the full data. 

### Handling Packet Loss
If the server receives a data request with the *packet loss bit* set to *0*, it SHOULD increase the size of the next congestion window. If the server receives a data request with the *packet loss bit* set to *1*, it SHOULD decrease the size of the next congestion window.
If the server does not receive a new data request after completely filling up a congestion window, it MAY close the connection after waiting a period of time.

## Timeouts {#Timeouts}

If not otherwise specified, all timeouts used in RFT, clients and servers MAY choose appropriate values depending on their specific environment and network.

# Checksum {#Checksum}
File checksums are computed using the SHA-256 algorithm over the whole file. [@RFC6234]

The checksum may be especially useful if the client wants to resume a file transfer from an incomplete state. The checksum can indicate if the file changed in-between the last and the current transfer.

# Message Formats {#MessageFormats}
All messages are transmitted using UDP and MUST be in network byte order ("big endian").

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

The *size* field of the header specifies the length of the RFT packet including its header in bytes. *Number of CWND slots*, *size of CWND* and the *packet loss bit* are used as defined in (#CongestionControl). The field *MsgType* is a constant indicating what kind of packet is following the header. The possible types are:

- 0x00: File-Request
- 0x01: Response to File-Request
- 0x02: Data Request
- 0x03: Data

The *number of options* field announces the amount of options following the header, before the packet data follows. Each option is encoded as Type-Length-Value encoded option using one byte for the option type, one byte for the option length. Currently there are no global option types registered.

## File Request
Sent by the client.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|       Number of files         |           Reserved            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              | Path name (length-many byte) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              | Path name (length-many byte) ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                            ...                                |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

A File Request contains the number of requested files in its first two bytes. After another reserved (for future use) two bytes, each requested file has a section containing the length of the path linking to the file and the path itself. The path's MUST follow the format defined in Section 3.3. of [@RFC3986].

## Response to File Request
Sent by the server. Presented message structure can be repeated for multiple files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|     File Index                |          Error code           |
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

When responding to a File-Request, the server sends a packet, that acknowledges the requested files in a packet that contains one section per file. The first field in each files section is a file index, that points to the position of that file in the array of files, as it was requested in the corresponding File-Request. The next two bytes contain an error code, that MUST contain one of these error codes:

- 0x00: all OK
- 0x01: file not found
- 0x02: temporarily not available
- 0x03: connection closed
- ...

For each file, that has the error code set to 0, the next fields contain the total file size and a checksum of 32 bytes of the total file.

## Request

Data requests and data packets use the same semantics for the fields as explained in the previous sections using the following structure:

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

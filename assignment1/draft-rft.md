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
This RFC describes the protocol "Robust File Transfer" (RFT) that defines the interaction between a server that offers files and clients that can download those files. The communication employs a binary protocol and messages are transferred over the network using UDP [@RFC0768].

RFT secures successful file transmissions even in the face of network problems such as message loss and reordering. The protocol is able to efficiently resume aborted transfers and to verify the integrity of transferred files. Transmission is performed with consideration towards the resource constraints of sender, receiver and network links.

This RFC is structured as follows. (#ProtocolFlow) gives a high-level intuition over the protocol's workings. (#FlowCongestion) explains the flow and congestion control mechanisms used by RFT. The message types used are defined in (#MessageFormats).


# Protocol flow {#ProtocolFlow}
An RFT server continuously listens for UDP packets at an address that is known to potential clients.

```
    Client                          Server
       +                              +
       |  Request files               |     - Setup phase
       | +--------------------------> |     .
       |                              |     .
       |  Respond with information    |     .
       |  about requested files       |     .
       | <--------------------------+ |     .
       |                              |     .
       |                              |     .
       |                              |     .
       |  Request byte range of file  |     - Data transmission phase
       | +--------------------------> |     .
       |                              |     .
       |                              |     .
       |  Send part of requested      |     .
       |  byte range.                 |     .
       | <--------------------------+ |     .
       |                              |     .
       |  Send part of requested      |     .
       |  byte range.                 |     .
       | <--------------------------+ |     .
       |                              |     .
       |                              |     .
       |  Request byte range of file  |     .
       | +--------------------------> |     .
       |                              |     .
       |               .              |     .
       |               .              |     .
       |               .              |     .
       +                              +
```

## File request
Connections are initiated by clients who send a single message with a list of the files that they want to fetch from the server. Each file is identified by a path that is resolved by the server. The position of each file in the request list determines the handle by which the file is referred to in the subsequent data request messages. For example: The file that appears first in the request list is assigned index 0, the next file index 1, and so on. 

The server replies with at least one message that contains the total size and checksum of each file that can be served. If a file can not be served, an explanatory error code is returned for that file.
<!-- Optionally, clients can request to not transfer the whole file but start the transfer from a specified byte offset. -->

## Data transfer
The client picks at least one of the desired files for which no error was returned. It sends a data request message that contains the corresponding index of that file and a byte range (the first message typically requests file 0 starting from byte 0).

The server receives the request and adds it to the request queue for that particular client. It starts fulfilling the request by sending parts of the requested byte range.

The client may need to renew its request before it has been fully served  as described in (#FlowCongestion). It MAY adapt the size of the requested byte range and the number of concurrently requested files for performance or other reasons.

## Transfer termination
Once the client has received a file in total, it MAY compute the checksum and compare it with the one given by the server at connection start (see (#Checksum)).

Once the client has received all desired files, it may terminate the transmission program. The server may clean up any state after a timeout.

## Resuming transfers
A file transmission may have been interrupted which results in that file being only partially written to disk. An RFT client may resume the transmission of that file by starting to request data not from byte zero but from an offset.

To provide the opportunity to resume a transfer, the client program SHOULD regularly store received data on a permanent storage medium.


# Flow and congestion control {#FlowCongestion}
A server needs to limit the amount of data it sends to not overwhelm the network or the receiving client. RFT uses the flow control mechanism described in (#FlowControl) to prevent overloading the receiver and the congestion control mechanism described in (#CongestionControl) to prevent overloading the available network resources.

## Flow Control {#FlowControl}
As described in (#ProtocolFlow), a client periodically requests a range of data from the server. The size of the requested data range MUST never be larger than the amount of data that the client is currently able to receive. The client SHOULD adapt the amount of concurrently requested data and its data timing to improve transmission speed.

A server MUST NOT send more data than a client requested. It is possible, that a server sends less than the requested amount of data, when the requested file is smaller than the requested amount of data or the server is otherwise limited, e.g., by congestion control. A server MUST only send requested byte ranges of requested files.

If, after sending all requested bytes, the server does not receive another data request from the client, even if the file has more bytes than requested, the server SHOULD close the connection after waiting for a period longer than a specified timeout (see (#Timeouts)).

## Congestion Control {#CongestionControl}
To avoid overloading the network that is used for data transfer, RFT uses a congestion control mechanism, that prevents the server from sending an overwhelming amount of data. The receiving client informs the server about packet loss which acts as a sign to reduce the data rate.

As long as no packet loss occurs, the server SHOULD slowly increase the sending rate.

```
Client (C)         Server (S)
   +                   +
   |  REQ F0 [00, 90]  |  C request byte range 0-90 of file 0
   | +---------------> |
   |                   |
   |  DAT F0 [00, 10]  |  S sets CWND size to 3.
   |  CWND 2/3         |  CWND free/maximum slots.
   | <---------------+ |
   |                   |
   |  DAT F0 [10, 20]  |
   |  CWND 1/3         |
   | <---------------+ |
   |                   |
   |  REQ F0 [20, 90]  |  S increases the congestion window
   | +---------------> |  to 4 for the next round.
   |                   |
   |  DAT F0 [20, 30]  |  The current congestion window is
   |  CWND 0/3         |  filled before the next is started.
   | <---------------+ |
   |                   |
   |  DAT F0 [30, 40]  |
   |  CWND 3/4         |
   | <---------------+ |
   |                   |
   |         .         |
   |         .         |
   |         .         |
   +                   +
```

### Detecting Packet Loss
Each data packet that a server sends is assigned to an instance of a congestion window ("CWND"). A congestion window has a certain size that is measured in "number of packets". In addition, the server keeps track of the number of free CWND slots; this number is decreased by one for each data packet that is sent to the client.

The server announces the size of the current congestion window in the *CWND size* field of the packet header. Additionally, it announces the number of packets that can follow in the same congestion window, before a new congestion window needs to be used, in the *free CWND slots* field of the data packet. A server MUST fill a congestion window, before opening a new one, i.e., a packet with zero *free CWND slots* MUST be send by the server.

A new congestion window is only opened if the server received a data request packet from the client since the start of the current congestion window. In other words, a server MUST receive a data request, before a new congestion window can be opened.

The client can infer whether some data packets were lost by monitoring the *CWND size* and *free CWND slots* fields in a data packet. The number of received packets for the current congestion window will eventually equal the announced congestion window size if no packets were lost. Otherwise, a gap in the *free CWND slots* sequence will have opened. To account for out-of-order packets, the client SHOULD wait a reasonable amount of time before deciding that packet loss has opened a gap in that sequence. The length of this time period MAY be a multiple of the average time measured between receiving two data packets.

If packet loss has been detected, the client MUST set the *MsgType* of the next data request to *0x03* (as described in (#Header)) and MUST re-request the missing data in a new data request, unless it decides to terminate the connection without receiving the full data.

### Handling Packet Loss
If the server receives a data request with the *MsgType* set to *0x02* (indicating no packet loss occured), it SHOULD increase the size of the next congestion window. If the server receives a data request with the *MsgType* set to *0x03*, it SHOULD decrease the size of the next congestion window.

If the server does not receive a new data request after completely filling up a congestion window, it MAY close the connection after waiting a reasonable amount of time.

## Timeouts {#Timeouts}
If not otherwise specified, all timeouts used in RFT may be chosen by clients and servers in a way to best adapt to the specific environment and network conditions.

# File checksums {#Checksum}
File checksums are computed using the SHA-256 algorithm [@RFC6234]. The whole file is fed to the digest computation. The client SHOULD compute the checksum of a file once its transfer is complete and compare it to the one supplied by the server. If the two checksums do not match, the file SHOULD be discarded.

A checksum may be especially useful if the client wants to resume a file transfer from an incomplete state. The checksum can indicate if the file changed in-between the last and the current transfer. Clients SHOULD therefore commit the checksum supplied by the server to permanent storage once a transmission starts.

# Message Formats {#MessageFormats}
All messages are transmitted using UDP and MUST be in network byte order ("big endian").

## Header {#Header}
All message types which are described in the following sections are prepended by this header.


```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Size (16 bit)                |    Msg Type   | # of options  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Option type  | Option length |   Option length-many byte    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Option type  | Option length |   Option length-many byte    ...
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

```

The *size* field of the header specifies the total length of the RFT packet including its header in bytes.

The field *MsgType* is a constant indicating what kind of packet is following the header. The possible types are:
- 0x00: File-Request
- 0x01: Response to File-Request
- 0x02: Data Request without indicating packet loss
- 0x03: Data Request indicating packet loss
- 0x04: Data

The *number of options* field announces the amount of options following the header, before the packet data follows. Each option is encoded as type-length-value (TLV) encoded option using one byte for the option type and one byte for the option length. Currently there are no global option types registered.

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

When responding to a File-Request, the server sends acknowledges the requested files by sending one instance of the presented section per file. The first field in each file section is the file index, that points to the position of that file in the array of files, as it was requested in the corresponding File-Request. The next two bytes contain an error code, that MUST contain one of these error codes:

- 0x00: all OK
- 0x01: file not found
- 0x02: temporarily not available
- 0x03: connection closed

For each file that has the error code set to 0, the next fields contain the total file size and a checksum of the total file (32 bytes large).

If a large number of files is requested, the file-response sections may not fit into on UDP packet. In this case, the server SHOULD send multiple response messages.

## Data Request
Data requests use the same semantics for the fields as explained in the previous sections. This message is only sent by the client to the server.

The following message structure is repeated one or more times to request multiple byte ranges, potentially of different files.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         File Index            |           Reserved            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of first requested byte (64 bit)        |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                Offset of last requested byte + 1 (64 bit)     |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             ...                               |
+                                                               +
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

The *Offset of first requested byte* points to the first requested byte of a range, while the *Offset of last requested byte* points to the first byte that is following the requested range.


## Data Packet
Data requests and data packets use the same semantics for the fields as explained in the previous sections using the following structure.

After the message header, a data packet begins with the following preamble that carries information used by congestion control.

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Number of free CWND slots    |         Size of CWND          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

*Number of CWND slots* and *size of CWND* are used as defined in (#CongestionControl). The CWND fields are only used by data packets sent by the server. 

The congestion control preamble is followed by one or more repetitions of the following data section.

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

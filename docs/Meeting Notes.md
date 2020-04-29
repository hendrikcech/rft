# 2020-04-29
- GitHub repository: hendrikcech/protocol-design
- Ordner pro Assignment
- RFC: markdown Dateien mit mmark in xml-Format und dann mit xml2rfc Tool in Ausgabeformat konvertieren
- go Struktur angelegt
- Spätestens nach Vorlesung am Montag wegen Congestion Control-Aufgabe reden
- Am Freitag an RFT arbeiten und ggf. telefonieren

## Code
- `rft/msgs.go`: eine Datei mit Nachrichtentypen und (Un-)Marshaling-Code
- `cmd/main.go`: CLI-Interface, das dann entweder Client oder Server ausführt
    - CLI cobra library
- `rft/client.go` und `rft/server.go`
- zwischen Client und Server geteilte Funktionalität in weitere Dateien auslagern
- Network Middleware, um Markov Chain und Logging zu implementieren. Könnte auch von Tests verwendet werden.

## CLI
- zusätzlicher Parameter, der Directory spezifiziert, in dem die Dateien liegen, die übertragen werden können; default: aktuelles Verzeichnis.

## Brainstorming Assignment 1
- What is a connection? Connection id; TCP quadrupel (IPs + ports)
    - DoS-Möglichkeit verhindern (Connection setup)
- Ordering: file-intern und über Dateien
- Granularity: bytes?
- https://en.wikipedia.org/wiki/Linear_network_coding
- Erstmal nur an Übertragung einer Datei denken und darauf dann Übertragung mehrere Dateien aufbauen?
- Reliability: ACKs
    - UDP hat Checksums, also werden korrupte Pakete automatisch verworfen
- Resume transfers
- Kodierung? Text oder binär
- [TCP Cwnd und Rwnd](https://blog.stackpath.com/glossary-cwnd-and-rwnd/)

## Attempt 1
1. Client fragt Dateien beim Server an: Nachricht C->S#1.

2. Server queued Dateien. Server fängt mit Datei 1 an und schickt die ersten 100 Byte an den Client: S->C#2
3. Client empfängt Daten. Client sended ACK (C->S#3).
4. Server wartet auf erstes ACK, um DOS zu verhindern. Server sended weitere Pakete in 100 Byte Teilen.
5. Client ACKed mehrere Pakete auf einmal. `ACK` ist der letzte Byte-Index, der korrekt empfangen wurde. `NACK` ist eine Liste, in der Abschnitte sind, die nicht empfangen wurden.

## Nachrichten
## Header
- uint size
- uint messageType
- uint version

### C->S#1: Datei-Anfrage
- uint NumberOfFiles
- Pro Datei:
    - uint Offset, ab der die Übertragung gestartet werden soll (resume transfers)
    - string Dateinamen

### S->C#2: Antwort auf Datei-Anfrage
- uint FileIndex; Index in der Liste, der Client-Anfrage (C->S#1); startet bei 0
- uint Offset
- uint Size: in byte
- Checksum:
    - Client merkt, wenn sich die Datei verändert hat (Transfer kann nicht wiederaufgenommen werden; dann C->S#4); muss für finale Validierung der Übertragung genutzt werden.

### S->C#3: Daten
- uint File
- uint Offset

### C->S#3: Acknowledgement
- uint FileIndex
- uint ReceivedUpToByte
- list of byte ranges NotReceived: [(start byte - end byte)]

### C->S#3: Beendigung der Datei-Verbindung
- uint FileIndex

## Fehlerfälle
### Angefragte Datei nicht gefunden
Server antwortet mit "File not found" Nachricht. Sollen, wenn mehrere Dateien angefragt wurden, die anderen Dateien übermittelt werden?

### Client-seitige Fehler
Abbruch durch Nutzer, kein freier Speicherplatz, etc.

-> Server erhält keine ACKs mehr und bricht die Übertragung nach Time-Out ab. 

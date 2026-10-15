# Heliotherm web control protocol

This describes how homeControl 2.1 (and heliocontrol) talk to a Heliotherm heat
pump with the *web control* controller. It was recovered by decompiling
homeControl (`HeliothermDLL.dll`, `DataFrame.dll`). The frame codec was checked
byte for byte against the original `DataFrame.dll`, and the reply formats were
captured from a real controller (firmware 2.07D) using read-only commands.

**This is not Modbus.** The controller speaks a proprietary ASCII command
language inside a small binary envelope, over:

- **TCP port 4001** of the Heliotherm (W)LAN modem, or
- **RS232** "Modem COM" (-X26) with a null-modem cable, 19200 baud, 8N1.

There is **no authentication and no encryption**. `~LIN;` takes no password.
Do not expose port 4001 to the internet.

## Frame

```
02 FD | dst | src | flags | cmp | len | payload (escaped) | crc
```

| Byte | Meaning |
|---|---|
| `02 FD` | Start of frame |
| `dst` / `src` | Addresses: heat pump `D0`, PC `E0` |
| `flags` | Bit 0/1: `len` was XON/XOFF; bit 2/3: `crc` was XON/XOFF (the byte is then sent as `00`) |
| `cmp` | Compression, always 0 (homeControl's RLE is never used) |
| `len` | Unescaped payload length, max 255. Longer messages are split into several frames |

**Payload escaping:**
- `02` and `2B` are followed by a stuffing `00`.
- `11` (XON), `13` (XOFF) and `FE` are sent as `FE 12`, `FE 14` and `FE 00`.

**Checksum:** `crc = crc ^ b ^ (b << 1)` (8 bit), starting at 0, over:
- `02`, `FD`
- `dst|0x80`, `src|0x80`
- `cmp`, `len`
- the unescaped payload

Example: `~LIN;` → `02 fd d0 e0 00 00 05 7e 4c 49 4e 3b 4c`

Text is ISO-8859-1 (point names contain umlauts).

## Session

| Command | Reply | Notes |
|---|---|---|
| `~LIN;` | `~OK;` | Login. homeControl repeats it after 25 s without traffic (keep-alive). If there is no answer, it sends `\r\nCONNECT 19200\r\n` unframed (analog/GSM modems) and retries. |
| `~LOUT;` | `~OK;` | Logout |
| `~RID;` | `~RID,123456;` | Controller ID ("Liegenschaft", also SP 3) |

## Points

The controller has *measuring points* (MP, sensors and outputs, 96 on 2.07D) and
*setup points* (SP, parameters, 392).

| Command | Reply |
|---|---|
| `~MP,NR=0;` | `~MP,NR=0,ID=0,NAME=Temp. Aussen,LEN=4,TP=1,BIT=1,VAL=15.3,MAX=40.0,MIN=-20.0,ERF=0,ORV=5.0,ORF=0,TRF=0,TRT=1,TRHV=1.0,TRI=100,TE=04.11.13-11:59:00,OFFV=0.0,RT1=0,RTL=0,WR=1,US=1;` |
| `~SP,NR=13;` | `~SP,NR=13,ID=13,NAME=Betriebsart,LEN=1,TP=0,BIT=0,VAL=1,MAX=7,MIN=0,WR=1,US=1;` |
| `~RM;` / `~RS;` | `~SUM=96;`, then one `~MP`/`~SP` reply per point (streamed) |
| `~MR,0,3,4;` | One `~MA,<id>,<value>,<?>;` per point (fast multi-read) |
| `~SP,NR=69,VAL=21.5;` | The point, with the stored value (**write**) |
| `~MP,NR=n,ORF=1,ORV=v;` / `~MP,NR=n,ORF=0;` | Force / release a measuring point (**write**) |

**Field reference:**

| Field | Meaning |
|---|---|
| `VAL`, `MIN`, `MAX` | Value and limits |
| `US` | Used in this installation |
| `WR` | Writable |
| `ACS` | Access level |
| `ERF` | Alarm outside limits |
| `RT1` | Call remote service on alarm |
| `TRF` | Trend recording enabled |
| `TRT` | Trend type: 0 interval, 1 hysteresis |
| `TRI` | Trend interval (s) |
| `TRHV` | Trend hysteresis |
| `TE` | Trend end (`dd.MM.yy-HH:mm:ss`) |

## Installation and pages

- **SP 9** carries the firmware version as its name (`2.07D`) and the installation bit mask as its value:

| Bits | Meaning |
|---|---|
| `0x1`, `0x2`, `0x4`, `0x8` | Energy source: brine, water, ground (direct expansion), air |
| `0x2000` | Mixer circuit 1 |
| `0x10`, `0x20`, `0x100`, `0x400`, `0x200`, `0x800` | Plant diagram variant 1..10 (buffer, hot water, fresh water station) |

- **SP 199 / SP 222:** mixer circuit 1 / 2 enabled.
- **SP 263:** energy metering.
- `internal/catalog` holds the full point-to-page map that homeControl used.

## Important points (firmware 2.07D)

| Point | Meaning |
|---|---|
| MP 0 / 3 / 4 | Outdoor / flow / return temperature |
| MP 2 | Hot water temperature |
| MP 57 | Return target |
| MP 31 | Fault |
| MP 56 | Compressor request |
| SP 13 | Operating mode: 0 off, 1 automatic, 2 cooling, 3 summer, 4 continuous, 5 reduced, 6 holiday, 7 party (8–10 on other firmware) |
| SP 14 | Fault reset (write 1) |
| SP 69 | Room target |
| SP 71 / 72 | Boost / reduction offset (K) |
| SP 76 | Heating limit |
| SP 80 / 81 / 82 | Return target at the heating limit / 0 °C / −15 °C (heating curve) |
| SP 83 / 85 | Hot water normal / minimum temperature |
| SP 171–173 | Compressor operating hours (hot water / heating / total) |

## Clock

| Command | Reply |
|---|---|
| `~CLK;` | `~CLK,DA=15.10.26,TI=12:55:12,WD=4;` |
| `~CLK,DA=dd.MM.yy,TI=HH:mm:ss,WD=n;` (**write**, WD: Mon=1 … Sun=7) | The same line echoed |

## Trend buffer

| Command | Reply / effect |
|---|---|
| `~TRS;` | `~SUM=1256;` |
| `~TR,0,1,2;` | `~TA,<record>,<mp>,dd.MM.yy-HH:mm:ss,<value>,<reason>;` per record. homeControl reads at most 10 per request. |
| `~TF,1;` / `~TF,0;` | Pause / resume recording (**write**) |
| `~TRD;` | Delete the buffer (**write**) |
| `~MP,NR=n,ERF=,TRF=,TRT=,TRHV=,TRI=,TE=,RT1=;` | Trend configuration of a point (**write**) |

## Time programs

| Command | Reply / effect |
|---|---|
| `~PRL;` | `~SUM=5;`, then `~PRI<id>,NAME=Heizung,EAD=7,NOS=3,STE=15,NOD=7,ACS=0,US=1;` per program |
| `~PRI<id>;` | One program header |
| `~PRD<id>;` | The program header, then `EAD×NOD` events `~PRE,PR=0,DAY=0,EV=2,ST=1,BEG=10:00,END=11:00;` |
| `~PRE,PR=,DAY=,EV=;` | One event |
| `~PRE,PR=,DAY=,EV=,ST=,BEG=,END=;` | Store an event (**write**) |

**Program fields:**

| Field | Meaning |
|---|---|
| `EAD` | Events per day |
| `NOS` | Number of states: 2 = off/on, 3 = normal/reduced/boost |
| `STE` | Step in minutes |
| `NOD` | Days (Monday = 0) |
| `US` | Used |

Events cover a day without gaps from 00:00 to `24:00`. Unused trailing events are `24:00-24:00`.

## Reply ordering

Replies are not tagged with a request ID, and counted requests (`~RM`, `~RS`,
`~PRL`, `~PRD`) keep streaming records after the first reply. If you send a new
command too early, it receives those leftover records. heliocontrol therefore
matches every single-reply request to its expected prefix (`~MP,NR=3,`,
`~SUM`, `~PRE,PR=…`) and discards anything else.

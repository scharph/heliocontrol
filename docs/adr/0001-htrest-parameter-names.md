# Loxone API uses HtREST's paths and parameter names

The Loxone API copies a subset of [HtREST](https://github.com/dstrigl/HtREST)
(`/api/v1/param`, `/api/v1/fastquery`) and addresses values by the names in
htheatpump's default `htparams.csv` (`HKR Soll_Raum`, `Temp. Aussen`, …), not by
our catalog keys or the controller's own point names. Existing Loxone
configurations and the [LoxWiki guide](https://loxwiki.atlassian.net/wiki/spaces/LOX/pages/1583089663/Heliotherm+Open+Source)
match on those exact strings, so moving from HtREST only needs a new address.
The names are frozen: renaming one silently breaks users' Loxone command patterns.

## Considered Options

- **Catalog keys** (`room_target`): consistent with Home Assistant, but every Loxone setup would be rebuilt by hand.
- **Controller point names** (`NAME=`): differ between firmware versions, contain umlauts and are not unique.

# heliocontrol

A self-hosted replacement for Heliotherm homeControl that reads and changes the
values of a Heliotherm *web control* heat pump controller.

## Language

**Measuring point (MP)**:
A read-only value the controller measures or reports, numbered 0–95 (e.g. outdoor temperature).
_Avoid_: sensor, reading

**Setup point (SP)**:
A numbered controller value, mostly settings (e.g. hot water target), some of them writable.
_Avoid_: setting, register

**Temporary value**:
A setup point changed for a limited time, after which heliocontrol restores the original value.
_Avoid_: boost, override (in prose)

**Write budget**:
The maximum number of changes per setup point per day.
_Avoid_: rate limit

**Parameter**:
A measuring point or setup point addressed by its HtREST name (e.g. `HKR Soll_Raum` = SP 69), as Loxone uses it.
_Avoid_: param, data point name

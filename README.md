# NextMN-CP Lite
**NextMN-CP Lite** is an experimental Control Plane simulator designed to be used along with **NextMN-UE Lite** and **NextMN-gNB Lite** to mimic from an UPF point-of-view a 5G & beyond Control Plane and a RAN.

3GPP N1/N2 interfaces are not (and will not be) implemented, and Control Plane is minimalistic on purpose.

This allow to test N3 and N4 interfaces of an UPF, and in particular to test handover procedures.

If you don't need to use handover procedures, consider using [UERANSIM](https://github.com/aligungr/UERANSIM) along with a real Control Plane (e.g. [free5GC](https://github.com/free5GC)'s NFs) instead.

## Getting started
### Build dependencies
- golang
- make (optional)

### Build and install
Simply run `make build` and `make install`.

## Author
Louis Royer and the NextMN Contributors

## Licence
MIT

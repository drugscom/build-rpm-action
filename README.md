# Build RPM

This action builds RPM packages from spec.

## Inputs

### `yum-extras`

Install extra packages before building.


## Example usage

```yaml
uses: drugscom/build-rpm-action@v1
with:
  args: SPECS/mypackage.spec
  yum-extras: https://example.com/yumprivrepo-release.rpm
```

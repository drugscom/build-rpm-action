# Build RPM

This action builds RPM packages from spec.

## Inputs

### `yum-extras`

Install extra packages before building.

## Outputs

### `successful`

List of specs successfully built

## Example usage

```yaml
uses: docker://ghcr.io/drugscom/rpmbuild-action:1
with:
  args: SPECS/mypackage.spec
  yum-extras: https://example.com/yumprivrepo-release.rpm
```

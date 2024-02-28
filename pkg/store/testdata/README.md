# Store test data

The [txtar](https://pkg.go.dev/golang.org/x/tools/txtar) files in testdata are test cases.

## Format

The filenames in the txtar files are JSON strings that define the command to run on the file contents (test data).

The fields of the JSON are:

- **cmd:** the command to Run
- **status:** the expected status return
- **manager:** the name of the manager
- **force:** force the apply

Commands are:

- **apply:** apply the object
- **delete:** delete the object
- **create:** create the object
- **assert:** assert that the data matches the object in the store

Example commands:

- `{"cmd":"apply","manager":"m1"}` => apply contents as manager "m1"
- `{"cmd":"apply","manager":"m2", "status":409}` => apply contents as manager "m1" and expect error status 409 (conflict)
- `{"cmd":"assert"}` => check the current object eqauls file contents

## Example

Here's an example, where we apply an object, and then assert that the object matches.

```txt
-- {"cmd":"apply","manager":"m1"} --
apiVersion: dummy/v1
kind: DummyApplyObject
metadata:
  name: test
  account: test
spec:
  text: test
  object:
    text: test
    numbers: [1,2,3]
-- {"cmd":"assert"} --
apiVersion: dummy/v1
kind: DummyApplyObject
metadata:
  name: test
  account: test
  managedFields:
  - manager: m1
    fieldsType: FieldsV1
    fieldsV1:
      f:spec:
        f:text: {}
        f:object:
          f:text: {}
          f:numbers: {}
spec:
  text: test
  object:
    text: test
    numbers: [1,2,3]
```

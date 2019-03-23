Guestbook is a sample application that records visitors' messages and
displays a cloud banner with an administrative message. The main business
logic is written in a cloud-agnostic manner using MySQL, the generic blob
API, and the generic runtimevar API. All platform-specific code is set up by
[Wire](https://github.com/google/wire).

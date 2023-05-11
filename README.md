# Matrixfs

Matrixfs is a file service used to connect to a Matrix network
`go install github.com/altid/matrixfs@latest`

## Usage

`matrixfs [-conf] [-p <servicename>]

## Configuration

```
service=matrix address=matrix.chat auth=pass=hunter2
	user=foo@bar.com
	name=guest
	log=/home/guest/logs/matrix/
	ssl=simple
```

- service matches the given servicename (default "matrix")
- address is the homeserver for the remote Matrix host
- auth is the authentication method
    - pass will send the string following pass= as your password to the remote Matrix server
    - factotum uses a local factotum (Plan9, plan9port)
- user is the login email used for Matrix
- name is the nickname you require
-  log is the directory that stores chat logs. A special value of `none` can be used to bypass logging
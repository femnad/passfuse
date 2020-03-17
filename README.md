# passfuse

Mounts [Pass][pass] secrets as a filesystem. Fuse implementation courtesy of [jacobsa/fuse][fuse]

# Why

Helps with programs which need to get credentials from files, e.g. like credentials fields in Terraform provider/backend configurations.

# How

```
passfuse [--createmountpath] [--mountpath MOUNTPATH] [--passwordstorepath PASSWORDSTOREPATH] [--prefix PREFIX] [--unmountafter UNMOUNTAFTER]
```

Where the options are
* `--contentfiles`, `-C`: Mount files containing the secret content? (default: true)
* `--createmountpath`, `-c`: Create mount path if it doesn't exist? (default: true)
* `--firstlinefiles`, `-f`: Mount files containing first lines of secrets? (default: true)
* `--mountpath MOUNTPATH`, `-m`: Mount path (default: $HOME/.mnt/passfuse)
* `--passwordstorepath PASSWORDSTOREPATH`, `-s`: Password store path (default `""`; fallback to `pass`'s default)
* `--prefix PREFIX`, `-p`: a prefix for limiting the mounted passwords (optional)
* `--unmountafter UNMOUNTAFTER`, `-u`: Unmount after given seconds (default: `0`; don't unmount)

# Notes

* Content files are mounted with a suffix of `.contents` where first line files are mounted with a suffix of `.first-line`, both minus the `.gpg` suffix of the corresponding `pass` secret file.
* It is sometimes necessary to report the file size correctly, and not just a large enough value, as having trailing bytes which might trip up programs parsing the mounted files. In order to do that the file sizes are determined by decrypting the secrets in memory and counting the bytes in the output. Therefore, list operations where there are a large number of secrets in a directory might take a long time at first before the sizes are cached.

[pass]: https://www.passwordstore.org/
[fuse]: https://github.com/jacobsa/fuse

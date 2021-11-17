
<div align="center">
  <img src="./assets/logo.png" align="center"></img>
  <br/>
  <br/>
  <p><i>S3 Bulk Delete helps to empty s3 buckets</i>
  <br/>
  <i>It is much faster than delete through aws-cli which is sequential</i>
  </p>

  [Submit an Issue](https://github.com/hemachandsai/cloudwatch-log-aggregator/issues/new)

  [![Gitter](https://badges.gitter.im/s3-bulk-delete/community.svg)](https://gitter.im/s3-bulk-delete/community?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)
</div>
<hr/>

## What is this project for
 - Currently we can use aws cli to empty an s3 bucket. But is very slow because of the sequential nature
 - s3-bulk-delete uses list-objects(5,500 objects per second) and delete-objects(3500 objects per second per prefix) api using aws sdk to delete at much faster rate respecting aws s3 rate limit rules
 
## Demo
<div align="center">
    <img src="./assets/s3-bulk-delete-demo.gif"/>
</div>
<br/>


## How to use
- Download the latest binary from the [releases section](https://github.com/hemachandsai/s3-bulk-delete/releases) depending on the target platform
- Execute the download binary with desired flags
```
usage: ./s3-bulk-delete -aws-region {{region}} -bucket {{{bucketname}}

  -aws-region string
        AWS Region in which bucket exists. (Required)
  -bucket string
        Bucket name to be deleted. (Required)
```

## Examples
```
s3-bulk-delete-windows.exe -aws-region us-east-1 -bucket test
./s3-bulk-delete-linux -aws-region us-east-1 -bucket test-bucket
./s3-bulk-delete-darwin -aws-region us-east-1 -bucket test-bucket
```

## Future plans
- Migrate the project to use go modules

## How to contribute
Feel free to raise a PR with new features or fixing existing bugs

## License
See the [LICENSE](LICENSE.md) file for license rights and limitations (Apache2.0).
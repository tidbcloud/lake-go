# Golang Driver

Lake Golang driver accesses Lake distributions or TiDB Cloud Lake through [REST API](https://docs.tidbcloud.com/).

## DSN

```
lake://root:root@lake.tidbcloud.com:8000/default?sslmode=disable&debug=1
lake://root:root@lake.tidbcloud.com:8000/default?presigned_url_disabled=true&wait_time_secs=30
```

## Connection Parameters

| Parameter              | Description                                                                                                                | Default | DSN Example                                                                   |
|------------------------|----------------------------------------------------------------------------------------------------------------------------|---------|-------------------------------------------------------------------------------|
| user                   | Lake user name                                                                                                             | none    | lake://{user}:root@lake.tidbcloud.com:8000/                                   |
| password               | Lake user password                                                                                                         | none    | lake://root:{password}@lake.tidbcloud.com:8000/                               |
| sslmode                | Enable SSL                                                                                                                 | disable | lake://root:root@lake.tidbcloud.com:8000/default?sslmode=enable               |
| presigned_url_disabled | whether use presigned url to upload data, generally if you use local disk as your storage layer, it should be set as true   | false   | lake://root:root@lake.tidbcloud.com:8000/default?presigned_url_disabled=true  |
| wait_time_secs         | Restful query api blocking time, if the query is not finished, the api will block for wait_time_secs seconds               | 10      | lake://root:root@lake.tidbcloud.com:8000/hello_lake?wait_time_secs=10         |
| max_rows_in_buffer     | the maximum rows in server session buffer                                                                                  | 5000000 | lake://root:root@lake.tidbcloud.com:8000/default?max_rows_in_buffer=50000     |
| max_rows_per_page      | the maximum rows per page in response data body                                                                            | 100000  | lake://root:root@lake.tidbcloud.com:8000/default?max_rows_per_page=100000     |

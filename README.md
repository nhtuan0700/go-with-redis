## Viết một web server sử dụng gin go + redis với

1. API /login, để tạo session cho mỗi người đăng nhập, dùng redis để lưu session id, user name ấy
2. API /ping chỉ cho phép 1 người được gọi tại một thời điểm ( với sleep ở bên trong api đó trong 5s)
3. đếm số lượng lần 1 người gọi api /ping
4. rate limit mỗi người chỉ được gọi API /ping 2 lần trong 60s
5. 1 API /top/ trả về top 10 người gọi API /ping nhiều nhất
6. Dùng hyperloglog để lưu xấp sỉ số người gọi api /ping , và trả về trong api /count


## How to run
```sh
docker compose up -d

make start
```

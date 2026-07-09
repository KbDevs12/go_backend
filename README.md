# Backend - Booking Lapangan Futsal

Backend API untuk aplikasi booking lapangan futsal. Backend ini melayani mobile frontend untuk user/customer dan admin panel untuk pengelolaan booking, pembayaran, field, schedule, user, notification, report, dan admin account.

## Tech Stack

- Go
- Gin HTTP Framework
- PostgreSQL / Supabase Database
- Firebase Auth
- JWT
- QRIS
- Supabase Storage / S3-compatible storage
- Ngrok untuk development tunnel

## Struktur Folder

```txt
.
├── cmd
│   ├── server
│   │   ├── main.go
│   │   └── router.go
│   └── seedadmin
│       └── main.go
├── internal
│   ├── admin
│   ├── auth
│   ├── booking
│   ├── config
│   ├── database
│   ├── middleware
│   ├── notification
│   ├── payment
│   ├── storage
│   └── user
├── pkg
│   ├── firebase
│   ├── qris
│   └── response
├── Makefile
├── go.mod
└── go.sum
```

## Fitur Utama

### User / Customer

- Login menggunakan Firebase ID token.
- Ambil profile user.
- Update profile user.
- Lihat notifikasi.
- Simpan dan hapus FCM token.
- Lihat availability field berdasarkan tanggal.
- Buat booking.
- Lihat booking milik sendiri.
- Lihat detail booking.
- Cancel booking yang masih pending.
- Ambil QR pembayaran.
- Upload bukti pembayaran.

### Admin

- Login admin.
- Dashboard.
- Kelola booking.
- Kelola pembayaran.
- Konfirmasi dan reject pembayaran.
- Kelola field/lapangan.
- Kelola schedule khusus per field.
- Kelola user.
- Kelola notifikasi.
- Generate report harian dan range.
- Kelola admin account khusus superadmin.

## Schedule Field

Backend menggunakan tabel `schedules` untuk mengatur jadwal khusus per field dan per tanggal.

Default jam operasional field adalah:

```txt
08:00 - 23:00
```

Jika admin membuat schedule khusus untuk tanggal tertentu, backend akan memakai data dari tabel `schedules`.

Behavior schedule:

- Jika tidak ada schedule khusus, field memakai default `08:00-23:00`.
- Jika ada schedule dengan `open_time` dan `close_time`, availability dan booking mengikuti jam tersebut.
- Jika schedule memiliki `is_closed = true`, field dianggap tutup full day pada tanggal itu.
- Booking akan ditolak jika jam booking berada di luar schedule.
- Availability frontend akan menampilkan jam buka/tutup dari schedule.

## Requirements

- Go sesuai versi di `go.mod`
- PostgreSQL / Supabase database
- Firebase project
- Firebase service account JSON
- Ngrok account untuk development mode
- Supabase Storage/S3 credential jika memakai upload bukti pembayaran

## Environment Variables

Buat file `.env` di root project.

Contoh:

```env
SERVER_PORT=8080
APP_ENV=development

JWT_SECRET=change-this-secret
JWT_EXPIRY_HOURS=24
INACTIVITY_LOGOUT_DAYS=30

DATABASE_URL=postgres://USER:PASSWORD@HOST:PORT/DATABASE

FIREBASE_CREDENTIALS_PATH=./firebase.json
FIREBASE_PROJECT_ID=your-firebase-project-id
FIREBASE_WEB_API_KEY=your-firebase-web-api-key

QRIS_STATIC=your-static-qris-value

SUPABASE_S3_ENDPOINT=your-supabase-s3-endpoint
SUPABASE_S3_REGION=ap-southeast-1
SUPABASE_S3_ACCESS_KEY_ID=your-access-key
SUPABASE_S3_SECRET_ACCESS_KEY=your-secret-key
SUPABASE_S3_BUCKET=payment-proofs
SUPABASE_STORAGE_PUBLIC_BASE_URL=your-public-storage-url

NGROK_AUTHTOKEN=your-ngrok-token
NGROK_RESERVED_DOMAIN=your-ngrok-reserved-domain
```

## Catatan APP_ENV

Jika:

```env
APP_ENV=development
```

Server akan berjalan di:

```txt
http://localhost:8080
```

Dan otomatis membuat tunnel Ngrok. Karena itu `NGROK_AUTHTOKEN` wajib tersedia.

Jika:

```env
APP_ENV=production
```

Server langsung listen ke port dari `SERVER_PORT`.

## Instalasi

```bash
go mod tidy
```

## Menjalankan Server

Dengan Makefile:

```bash
make run
```

Atau langsung:

```bash
go run ./cmd/server
```

## Build

```bash
make build
```

Output binary akan masuk ke:

```txt
./bin/main
```

## Test

```bash
go test ./...
```

Atau jika hanya ingin test QRIS:

```bash
go test ./pkg/qris
```

## API Routes

### Health

```http
GET /health
```

### Public

```http
POST /api/v1/auth/login
POST /api/v1/auth/admin-login
GET  /api/v1/fields/availability?date=YYYY-MM-DD
```

### Customer

Semua route customer membutuhkan header:

```http
Authorization: Bearer <token>
```

Routes:

```http
POST   /api/v1/auth/refresh

GET    /api/v1/user/profile
PATCH  /api/v1/user/profile
GET    /api/v1/user/notifications
POST   /api/v1/user/fcm-token
DELETE /api/v1/user/fcm-token

POST   /api/v1/bookings
GET    /api/v1/bookings
GET    /api/v1/bookings/:id
PATCH  /api/v1/bookings/:id/cancel

GET    /api/v1/payments/:booking_id/qr
POST   /api/v1/payments/:booking_id/proof
```

### Admin

Semua route admin membutuhkan token admin:

```http
Authorization: Bearer <admin-token>
```

Routes:

```http
GET    /api/v1/admin/dashboard

GET    /api/v1/admin/bookings
POST   /api/v1/admin/bookings
GET    /api/v1/admin/bookings/:id
PATCH  /api/v1/admin/bookings/:id/status

GET    /api/v1/admin/payments
GET    /api/v1/admin/payments/:booking_id
POST   /api/v1/admin/payments/:booking_id/confirm
POST   /api/v1/admin/payments/:booking_id/reject

GET    /api/v1/admin/reports/daily
GET    /api/v1/admin/reports/range

GET    /api/v1/admin/notifications

GET    /api/v1/admin/users
GET    /api/v1/admin/users/:id
PATCH  /api/v1/admin/users/:id
DELETE /api/v1/admin/users/:id

GET    /api/v1/admin/fields
POST   /api/v1/admin/fields
GET    /api/v1/admin/fields/:id
PATCH  /api/v1/admin/fields/:id
DELETE /api/v1/admin/fields/:id

GET    /api/v1/admin/fields/:id/schedules
PUT    /api/v1/admin/fields/:id/schedules
DELETE /api/v1/admin/fields/:id/schedules/:date
```

### Superadmin

Routes berikut hanya untuk role `superadmin`:

```http
GET    /api/v1/admin/admins
POST   /api/v1/admin/admins
PATCH  /api/v1/admin/admins/:id
DELETE /api/v1/admin/admins/:id
```

## Schedule API

### List Schedule Field

```http
GET /api/v1/admin/fields/:id/schedules
```

Optional query:

```txt
from=YYYY-MM-DD
to=YYYY-MM-DD
```

Contoh:

```http
GET /api/v1/admin/fields/field-id/schedules?from=2026-05-01&to=2026-05-31
```

### Tambah / Update Schedule Field

```http
PUT /api/v1/admin/fields/:id/schedules
```

Body:

```json
{
  "date": "2026-05-22",
  "open_time": "08:00",
  "close_time": "23:00",
  "is_closed": false
}
```

Untuk menutup field full day:

```json
{
  "date": "2026-05-22",
  "open_time": "08:00",
  "close_time": "23:00",
  "is_closed": true
}
```

### Hapus Schedule Override

```http
DELETE /api/v1/admin/fields/:id/schedules/:date
```

Contoh:

```http
DELETE /api/v1/admin/fields/field-id/schedules/2026-05-22
```

Setelah schedule override dihapus, field akan kembali memakai jam default `08:00-23:00`.

## Format Response

Response API memakai format umum:

```json
{
  "success": true,
  "message": "request berhasil",
  "data": {}
}
```

Untuk error:

```json
{
  "success": false,
  "message": "pesan error",
  "data": null
}
```

## Database Utama

Tabel utama yang digunakan:

- `users`
- `admins`
- `fields`
- `schedules`
- `bookings`
- `payments`
- `notifications`
- `reports`
- `user_device_tokens`
- `admin_device_tokens`
- `admin_notifications`

## Flow Booking

1. User login via Firebase.
2. Frontend mengirim Firebase ID token ke backend.
3. Backend membuat JWT session.
4. User memilih tanggal booking.
5. Frontend request availability.
6. Backend membaca field, booking aktif, dan schedule khusus.
7. User memilih field dan jam.
8. Backend validasi:
   - field tersedia,
   - tanggal valid,
   - jam booking valid,
   - field tidak closed,
   - jam booking berada dalam schedule,
   - slot tidak overlap dengan booking lain.
9. Booking dibuat dengan status `pending_payment`.
10. Payment record otomatis dibuat.
11. User mengambil QR pembayaran.
12. User upload bukti pembayaran.
13. Admin melakukan confirm atau reject pembayaran.

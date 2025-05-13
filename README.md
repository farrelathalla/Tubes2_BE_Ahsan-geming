# 🧪 Algoritma BFS dan DFS pada Little Alchemy 2
# Tugas Besar 2 Strategi Algoritma IF2211

![Desain tanpa judul (2)](https://github.com/user-attachments/assets/60895446-f17a-4e66-858a-4eca0b5ec754)

## 📌 Deskripsi  
Repository ini berisi **backend** untuk **Little Alchemy 2 Finder**, yang memungkinkan pengguna mencari elemen menggunakan tiga algoritma pencarian: **Breadth-First Search (BFS)**, **Depth-First Search (DFS)**, dan **Bidirectional Search (BDS)**. Backend ini bertanggung jawab untuk menangani permintaan, memproses pencarian, dan mengembalikan jalur untuk pembuatan elemen.

## 🛠 Struktur Program
Berikut adalah struktur program tugas kecil ini :
```sh
/Tubes2_BE_Ahsan-geming
├── /algorithms             # Kumpulan algoritma
│   ├── bfs.go     
│   ├── bidirectional.go             
│   └── dfs.go   
├── /scraper                # Scraper
│   └── scraper.go
├── Dockerfile
├── main.go                 # Program utama
└── README.md               # Dokumentasi projek
```

## Getting Started 🌐
Berikut instruksi instalasi dan penggunaan program

### Prerequisites

Pastikan anda sudah memiliki:
- **Golang 1.24 atau lebih baru**
- **IDE atau terminal** untuk menjalankan program

### Installation
1. **Clone repository ke dalam suatu folder**

```bash
  https://github.com/farrelathalla/Tubes2_BE_Ahsan-geming.git
```

2. **Nyalakan Docker Desktop**

3. **Pergi ke directory /Tubes2_BE_Ahsan-geming**

```bash
  cd Tubes2_BE_Ahsan-geming
```

4. **Compile program**

```bash
  docker build -t go-backend .
```

5. **Jalankan program**

```bash
  docker run -p 8080:8080 go-backend
```

## **📌 Cara Penggunaan**

1. **Jalankan program** melalui terminal atau IDE yang mendukung Golang.
2. Setelah menyalakan backend, **nyalakan frontend** lalu bermain!

## **✍️ Author**
| Name                              | NIM        |
|-----------------------------------|------------|
| Ahsan Malik Al Farisi             | 13523074   |
| Kefas Kurnia Jonathan             | 13523113   |
| Farrel Athalla Putra              | 13523118   |

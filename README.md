# Introduction
 Monitor wchich twitch streams are online or not directly from the terminal 

## How to run
### 1. Edit the list of streamers
Give a list of streamers inside **config.json**

### 2. Connect to twitch API

Retrieve your **TWITCH_CLIENT_ID** and **TWITCH_CLIENT_SECRET** from Twitch Developer Console : https://dev.twitch.tv/console

Go to Applications -> Register your Application -> Fill the form -> Create -> Retrieve your client ID and client Secret
<img width="950" height="143" alt="Capture d’écran 2025-08-29 à 19 15 08" src="https://github.com/user-attachments/assets/470eb125-d8ab-4d78-8427-6c5e53da1e0a" />

<img width="737" height="695" alt="Capture d’écran 2025-08-29 à 19 16 16" src="https://github.com/user-attachments/assets/74b88d84-960e-46b7-bf50-1bafaaf11f60" />

#### Set Env Variables For Linux/macOS
```
export TWITCH_CLIENT_ID="your_client_id"
export TWITCH_CLIENT_SECRET="your_client_secret"
```

### 3. Run the program 
```
go run main.go
```

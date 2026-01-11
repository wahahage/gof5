import { useState, useEffect } from 'react';
import './App.css';
import { Connect, Disconnect } from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";

function App() {
    const [server, setServer] = useState("");
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const [status, setStatus] = useState("Disconnected");
    const [logs, setLogs] = useState<string[]>([]);

    useEffect(() => {
        EventsOn("log", (msg: string) => {
            setLogs(prev => [msg.trim(), ...prev].slice(0, 1000));
        });

        EventsOn("status", (msg: string) => {
            setStatus(msg);
        });

        EventsOn("connect_error", (msg: string) => {
            setStatus("Error");
            setLogs(prev => [`ERROR: ${msg}`, ...prev]);
        });

        EventsOn("error", (msg: string) => {
            setStatus("Error");
            setLogs(prev => [`ERROR: ${msg}`, ...prev]);
        });

        const savedServer = localStorage.getItem("server");
        const savedUser = localStorage.getItem("username");
        if (savedServer) setServer(savedServer);
        if (savedUser) setUsername(savedUser);
    }, []);

    const handleConnect = async () => {
        if (!server || !username || !password) {
            alert("Please fill all fields");
            return;
        }

        localStorage.setItem("server", server);
        localStorage.setItem("username", username);

        // Clear logs on new connection
        setLogs([]);
        setStatus("Connecting...");

        try {
            await Connect(server, username, password);
        } catch (e: any) {
            setStatus("Error");
            setLogs(prev => [`Failed to start: ${e}`, ...prev]);
        }
    };

    const handleDisconnect = async () => {
        await Disconnect();
    };

    const isConnected = status === "Connecting..." || status === "Connected";

    return (
        <div id="app">
            <div className="header">
                <div className="title">GoF5 VPN</div>
                <div style={{ color: status === "Disconnected" || status === "Error" ? "#f7768e" : "#9ece6a", fontWeight: "bold" }}>
                    {status}
                </div>
            </div>

            <div className="form-container">
                <div className="input-group">
                    <label>Server</label>
                    <input
                        value={server}
                        onChange={(e) => setServer(e.target.value)}
                        placeholder="vpn.example.com"
                        disabled={isConnected}
                    />
                </div>
                <div className="input-group">
                    <label>Username</label>
                    <input
                        value={username}
                        onChange={(e) => setUsername(e.target.value)}
                        placeholder="user"
                        disabled={isConnected}
                    />
                </div>
                <div className="input-group">
                    <label>Password</label>
                    <input
                        type="password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        disabled={isConnected}
                    />
                </div>

                {(!isConnected) ? (
                    <button className="btn-connect" onClick={handleConnect}>
                        Connect
                    </button>
                ) : (
                    <button className="btn-disconnect" onClick={handleDisconnect}>
                        Disconnect
                    </button>
                )}
            </div>

            <div className="terminal">
                {logs.length === 0 && <div className="log-placeholder">Ready to connect...</div>}
                {logs.map((log, i) => (
                    <div key={i} className="log-line">{log}</div>
                ))}
            </div>
        </div>
    )
}

export default App

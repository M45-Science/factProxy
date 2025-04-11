package main

const pageTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Local Server Bridge – M45-Science</title>
  <meta content="width=device-width, initial-scale=1.0" name="viewport">
  <link href="m45.css" rel="stylesheet" type="text/css">
  <link href="icon.png" rel="shortcut icon" type="image/x-icon">
  <style>
    body {
      background-color: #333;
      color: #ddd;
      font-family: 'Arial', sans-serif;
      margin: 0;
      padding: 0;
      text-align: center;
    }
    header {
      background-color: #222;
      padding: 20px;
      box-shadow: 0 4px 8px rgba(0, 0, 0, 0.6);
    }
    header h1 {
      color: #fff;
      font-size: 2em;
      margin: 0;
    }
    header h2 {
      color: #bbb;
      font-style: italic;
      margin-top: 5px;
    }
    .server-list {
      display: flex;
      flex-wrap: wrap;
      justify-content: center;
      padding: 20px;
      gap: 15px;
    }
    .server {
      background-color: #444;
      padding: 15px;
      width: 160px;
      border-radius: 10px;
      box-shadow: 0 4px 8px rgba(0, 0, 0, 0.4);
      transition: transform 0.2s;
    }
    .server:hover {
      box-shadow: 0 6px 12px rgba(0, 0, 0, 0.6);
    }
    .server h3 {
      margin: 0 0 10px;
      color: #ddd;
    }
    .server a {
      display: inline-block;
      background-color: #666;
      padding: 8px 12px;
      border-radius: 6px;
      color: #ddd;
      text-decoration: none;
      font-weight: bold;
      transition: background-color 0.2s;
    }
    .server a:hover {
      background-color: #888;
      color: #fff;
    }
    footer {
      background-color: #222;
      color: #888;
      padding: 10px;
      font-size: 0.9em;
      margin-top: 30px;
    }
  </style>
</head>
<body>
  <header>
    <h1>M45-Science</h1>
    <h2>LOCAL-bridge connect links:</h2>
  </header>

  <div class="server-list">
    {{range .Servers}}
      <div class="server">
        <h3>{{.Name}}</h3>
        <a href="steam://run/427520//--mp-connect%20127.0.0.1:{{.Port}}/">Connect</a>
      </div>
    {{end}}
  </div>
</body>
</html>
`

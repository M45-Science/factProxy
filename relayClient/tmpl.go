package main

const pageTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Local Server Bridge – M45-Science</title>
  <meta content="width=device-width, initial-scale=1.0" name="viewport">
  <!-- Example of optional external stylesheet or favicon references -->
  <link href="m45.css" rel="stylesheet" type="text/css">
  <link href="icon.png" rel="shortcut icon" type="image/x-icon">
  
  <style>
    :root {
      /* Accent color: adjust to taste */
      --accent-color: #ff6f3f;
      --bg-gradient-start: #1b1b1b;
      --bg-gradient-end: #333333;
      --header-bg: #222222;
      --box-bg: #444444;
      --text-color: #dddddd;
      --secondary-text-color: #bbbbbb;
    }

    /* Global Reset / Base */
    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    body {
      font-family: 'Arial', sans-serif;
      font-size: 16px;
      line-height: 1.5;
      text-align: center;
      color: var(--text-color);
      background: linear-gradient(135deg, var(--bg-gradient-start), var(--bg-gradient-end));
      margin: 0;
      padding: 0;
    }

    /* Header */
    header {
      background-color: var(--header-bg);
      padding: 2rem 1rem 1rem;
      box-shadow: 0 4px 8px rgba(0, 0, 0, 0.6);
    }
    header .logo {
      max-width: 120px;
      height: auto;
      margin-bottom: 1rem;
    }
    header h1 {
      font-size: 2.2em;
      color: #ffffff;
      margin-bottom: 0.2rem;
      text-transform: uppercase;
      letter-spacing: 1px;
    }
    header h2 {
      color: var(--secondary-text-color);
      font-style: italic;
      font-weight: normal;
      margin-bottom: 1rem;
    }

    /* Server List */
    .server-list {
      display: flex;
      flex-wrap: wrap;
      justify-content: center;
      gap: 20px;
      padding: 2rem 1rem;
      max-width: 1200px;
      margin: 0 auto;
    }
    .server {
      background-color: var(--box-bg);
      width: 180px;
      min-height: 120px;
      border-radius: 10px;
      box-shadow: 0 4px 8px rgba(0, 0, 0, 0.4);
      transition: transform 0.2s, box-shadow 0.2s;
      display: flex;
      flex-direction: column;
      justify-content: center;
      align-items: center;
      padding: 1.2rem;
    }
    .server:hover {
      transform: translateY(-2px);
      box-shadow: 0 6px 12px rgba(0, 0, 0, 0.6);
    }
    .server h3 {
      margin-bottom: 1rem;
      color: var(--text-color);
      font-size: 1.1rem;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    .server a {
      background-color: var(--accent-color);
      padding: 0.6rem 1rem;
      border-radius: 6px;
      color: #ffffff;
      text-decoration: none;
      font-weight: bold;
      transition: background-color 0.2s;
    }
    .server a:hover {
      background-color: #ff8b60; /* Slightly lighter shade for hover */
    }

    /* Footer */
    footer {
      background-color: var(--header-bg);
      color: var(--secondary-text-color);
      padding: 0.8rem;
      margin-top: 2rem;
      font-size: 0.9rem;
      box-shadow: 0 -4px 8px rgba(0, 0, 0, 0.6);
    }
  </style>
</head>
<body>
  <header>
    <!-- Logo -->
    <img class="logo" src="https://m45sci.xyz/m45-2024b-128.png" alt="M45 Logo" />
    <h1>M45-Science</h1>
    <h2>Local Bridge Connect Links:</h2>
  </header>

  <div class="server-list">
    {{range .Servers}}
      <div class="server">
        <h3>{{.Name}}</h3>
        <a href="steam://run/427520//--mp-connect%20127.0.0.1:{{.Port}}/">Connect</a>
      </div>
    {{end}}
  </div>

  <footer>
    &copy; 2025 M45-Science
  </footer>
</body>
</html>
`

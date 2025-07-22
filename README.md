# 🤖 Scope Automatic Bug Bounty (SABB)

This script is designed to automate the download of the scope for programs listed on the **HackerOne** platform 🕵️‍♂️. The scope defines which assets, domains, and endpoints are authorized for security testing.

By using this tool, security researchers can save hours of manual work, getting all the scope information in a structured format ready to be used in their reconnaissance and scanning workflows.

---
### 🛠️ Installation

To install the tool, make sure you have Go installed and run the following command:

```bash
go install -v [github.com/betillogalvanfbc/sabb@latest](https://github.com/betillogalvanfbc/sabb@latest)
```


🚀 Usage
Once installed, you can use it as follows, replacing the placeholders:

sabb -program hackerone -apikey <YOUR_API_KEY> -username <YOUR_USERNAME> -timeout 2m

-program: The platform to use (in this case, hackerone).

-apikey: Your personal HackerOne API token.

-username: Your HackerOne username.

-timeout: The maximum request timeout.


🔑 Getting Your HackerOne API Key
Generate Your Token: First, you need to create an API token in your HackerOne account settings.

🔗 Direct Link: https://hackerone.com/settings/api_token/edit

Save Your Token: Copy and save the generated token. You will need to pass it to the script using the -apikey parameter.


📚 References
For more information on how the HackerOne API works, check the official documentation:

API Documentation: https://api.hackerone.com/getting-started-hacker-api/



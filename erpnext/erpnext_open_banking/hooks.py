# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


app_name = "erpnext_open_banking"
app_title = "ERPNext Open Banking"
app_publisher = "open-banking.io"
app_description = "Connect your bank accounts to ERPNext via open-banking.io — no eIDAS certificates required."
app_icon = "octicon octicon-repo"
app_color = "#29CDFF"
app_email = "info@open-banking.io"
app_license = "MIT"

# --------------------------------------------------------------------------- #
# Required apps
# --------------------------------------------------------------------------- #
required_apps = ["erpnext"]

# --------------------------------------------------------------------------- #
# Doctype JS — inject "Sync Now" button on Bank Account form
# --------------------------------------------------------------------------- #
doctype_js = {
    "Bank Account": "public/js/bank_account.js",
}

# --------------------------------------------------------------------------- #
# Scheduler — periodic bank transaction sync (every 6 hours)
# --------------------------------------------------------------------------- #
scheduler_events = {
    "cron": {
        "0 */6 * * *": [
            "erpnext_open_banking.erpnext_open_banking.utils.connector.sync_all_connections"
        ],
    },
}

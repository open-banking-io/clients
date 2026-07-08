# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


import frappe
from frappe import _
from frappe.model.document import Document


class OpenBankingSettings(Document):
    pass


@frappe.whitelist(methods=["POST"])
def test_connection():
    """Validates the credentials bundle by calling the OBI API.

    Called from the Settings form's 'Test Connection' button.
    """
    from erpnext_open_banking.erpnext_open_banking.utils.connector import get_client

    frappe.only_for(("Accounts Manager", "System Manager"))

    doc = frappe.get_single("Open Banking Settings")
    if not doc.get_password("credentials_bundle", raise_exception=False):
        return {"success": False, "message": _("No credentials bundle provided.")}

    try:
        client = get_client()
        connections = client.get_connections()
        client.close()
        return {
            "success": True,
            "message": _(
                "Connected successfully. {0} bank connection(s) found."
            ).format(len(connections)),
        }
    except Exception as exc:
        return {"success": False, "message": str(exc)}

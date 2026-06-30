# ERPNext Open Banking © 2026
# Author: open-banking.io
# Licence: MIT


import frappe
from frappe import _
from frappe.model.document import Document


class OpenBankingSettings(Document):
    pass


@frappe.whitelist()
def test_connection():
    """Validates the credentials bundle by calling the OBI API.

    Called from the Settings form's 'Test Connection' button.
    """
    from erpnext_open_banking.erpnext_open_banking.utils.client import OpenBankingClient

    doc = frappe.get_single("Open Banking Settings")
    if not doc.credentials_bundle:
        return {"success": False, "message": _("No credentials bundle provided.")}

    try:
        client = OpenBankingClient.from_credentials(doc.credentials_bundle)
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

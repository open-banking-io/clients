// Open Banking Settings — client-side test-connection button

frappe.ui.form.on("Open Banking Settings", {
    refresh(frm) {
        frm.add_custom_button(__("Test Connection"), function () {
            frappe.call({
                method:
                    "erpnext_open_banking.erpnext_open_banking.doctype.open_banking_settings.open_banking_settings.test_connection",
                freeze: true,
                freeze_message: __("Testing connection..."),
                callback: function (r) {
                    if (r.message) {
                        if (r.message.success) {
                            frappe.msgprint({
                                title: __("Success"),
                                message: r.message.message,
                                indicator: "green",
                            });
                        } else {
                            frappe.msgprint({
                                title: __("Error"),
                                message: r.message.message,
                                indicator: "red",
                            });
                        }
                    }
                },
            });
        });
    },
});

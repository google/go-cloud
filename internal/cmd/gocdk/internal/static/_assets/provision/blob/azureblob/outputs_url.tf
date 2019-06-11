    BLOB_BUCKET_URL       = "${local.azureblob_bucket_url}"
    AZURE_STORAGE_ACCOUNT = "${azurerm_storage_account.storage_account.name}"
    AZURE_STORAGE_KEY     = "${azurerm_storage_account.storage_account.primary_access_key}"


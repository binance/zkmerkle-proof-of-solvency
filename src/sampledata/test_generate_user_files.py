import csv
import os
import random
import tempfile
import unittest
from unittest.mock import patch

import generate_user_files as generator


class ValidAccountGenerationTests(unittest.TestCase):
    def setUp(self):
        self.original_cwd = os.getcwd()
        self.tempdir = tempfile.TemporaryDirectory()
        os.chdir(self.tempdir.name)
        self.addCleanup(self._cleanup_tempdir)

        # One USDT is represented by 1e8 for the normal-token fixtures.
        # SHIB deliberately has no collateral tiers, matching the repository CSV.
        self.token2price = {
            "btc": 10**8,
            "eth": 10**8,
            "bnb": 10**8,
            "shib": 1797000000,
        }
        supported_tiers = [[10**30, 100]]
        self.token2tiers = {
            "btc": supported_tiers,
            "eth": supported_tiers,
            "bnb": supported_tiers,
            "shib": [],
        }

    def _cleanup_tempdir(self):
        os.chdir(self.original_cwd)
        self.tempdir.cleanup()

    def test_valid_accounts_use_per_asset_collateral_and_report_net_balances(self):
        random.seed(7)
        with patch.object(
            generator,
            "read_cex_data",
            return_value=(
                self.token2price,
                self.token2tiers,
                self.token2tiers,
                self.token2tiers,
            ),
        ):
            generator.generate_data(0, 25, 0)

        with open("sample_users0.csv", newline="", encoding="utf-8") as csv_file:
            rows = list(csv.DictReader(csv_file))

        self.assertEqual(len(rows), 25)
        for row in rows:
            for symbol in ("btc", "eth", "bnb", "shib"):
                equity = float(row[f"e_{symbol}"])
                debt = float(row[f"d_{symbol}"])
                net_balance = float(row[symbol])

                self.assertGreaterEqual(debt, 0.0, symbol)
                self.assertLessEqual(
                    debt,
                    equity + 10 ** -6,
                    f"{symbol} debt must not exceed its equity",
                )
                self.assertAlmostEqual(
                    net_balance,
                    equity - debt,
                    places=6,
                    msg=f"{symbol} net balance must equal equity minus debt",
                )

            # SHIB has no configured collateral tiers. It must not inherit
            # collateral value from BTC, ETH, or BNB.
            self.assertEqual(float(row["d_shib"]), 0.0)


if __name__ == "__main__":
    unittest.main()

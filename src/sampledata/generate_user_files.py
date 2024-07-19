import csv
import os
import sys
from multiprocessing import Process
import random

SpecialToken = {"shib": True}
SpecialTokenPriceMultiplier = pow(10, 14)
SpecialTokenNumMultiplier = pow(10, 2)
NormalTokenPriceMultiplier = pow(10, 8)
NormalTokenNumMultiplier = pow(10, 8)
TokenValueMultiplier = pow(10, 16)

def read_cex_data(file_name):
    with open(file_name, "r") as csv_file:
        reader = csv.reader(csv_file)
        data = list(reader)[1:]
    
    token2price = {}
    token2vltiersratio = {}
    token2margintiersratio = {}
    token2pmtiersratio = {}
    for d in data:
        token = d[0]
        if token in SpecialToken:
            token2price[token] = int(float(d[1])*SpecialTokenPriceMultiplier)
        else:
            token2price[token] = int(float(d[1])*NormalTokenPriceMultiplier)

        token2vltiersratio[token] = parse_tiers_ratio_data(d[2])
        token2margintiersratio[token] = parse_tiers_ratio_data(d[3])
        token2pmtiersratio[token] = parse_tiers_ratio_data(d[4])
    return token2price, token2vltiersratio, token2margintiersratio, token2pmtiersratio
    

def parse_tiers_ratio_data(data):
    if len(data) == 0:
        return []
    data = data.strip("[]").split(",")
    boundary2ratio = []
    for d in data:
        d = d.strip(" ").split(":")
        boundary2ratio.append([int(d[0].split("-")[1])*TokenValueMultiplier, int(d[1])])
    return boundary2ratio

def generate_data(id, num, invalid_num):
    token2price, token2vltiersratio, token2margintiersratio, token2pmtiersratio = read_cex_data("cex_assets_info.csv")
    content = [['rn', 'id', 'e_btc', 'd_btc', 'btc', 'vl_btc', 'm_btc', 'pm_btc',
              'e_eth', 'd_eth', 'eth', 'vl_eth', 'm_eth', 'pm_eth',
              'e_bnb', 'd_bnb', 'bnb','vl_bnb','m_bnb','pm_bnb', 
              'e_shib', 'd_shib', 'shib', 'vl_shib', 'm_shib', 'pm_shib', 'total_net_balance_usdt']]

    token_names = ["btc", "eth", "bnb", "shib"]
    total_tokens = 4
    for i in range(num):
        real_content = [i+id*num, "{0:0{1}x}".format(i+id*num, 64)]
        if i < invalid_num:
            # 0 means e_token < vl_token + m_token + pm_token
            # 1 means d_token * token_price > GetCollateralValue(vl_token * token_price) + GetCollateralValue(m_token * token_price) + GetCollateralValue(pm_token * token_price)
            # GetCollateralValue(token) is for getting the collateral value of token according to the collateral tiers ratio 
            invalid_type = i % 2
            if invalid_type == 0:
                for p in range(total_tokens):
                    roundPrecision = 8
                    if token_names[p] in SpecialToken:
                        roundPrecision = 2
                    equity = round(random.uniform(0, 1000), roundPrecision)
                    debt = round(equity / 2, roundPrecision)
                    vl_token = round(equity / 2, roundPrecision)
                    m_token = round(equity / 4, roundPrecision)
                    pm_token = round(equity / 2, roundPrecision)
                    real_content.extend([str(equity), str(debt), str(equity - debt), str(vl_token), str(m_token), str(pm_token)])
            elif invalid_type == 1:
                for p in token_names:
                    roundPrecision = 8
                    if p in SpecialToken:
                        roundPrecision = 2
                    if len(token2vltiersratio[p]) > 0:
                        v = token2vltiersratio[p][-1][0]/token2price[p]
                        if p in SpecialToken:
                            v = v / SpecialTokenNumMultiplier
                        else:
                            v = v / NormalTokenNumMultiplier
                        equity = round(random.uniform(0, v), roundPrecision)
                        vl_token = round(equity / 2, roundPrecision)
                        m_token = round(equity / 4, roundPrecision)
                        pm_token = round(equity / 8, roundPrecision)
                        debt_token_num = get_debt_token_num(p, vl_token, m_token, pm_token, token2vltiersratio, token2margintiersratio, token2pmtiersratio, token2price)
                        debt_token_num = round(debt_token_num * 1.01, roundPrecision)
                        real_content.extend([str(equity), str(debt_token_num), str(equity - debt_token_num), str(vl_token), str(m_token), str(pm_token)])
                    else:
                        equity = round(random.uniform(0, 1000), roundPrecision)
                        vl_token = round(equity / 2, roundPrecision)
                        m_token = round(equity / 4, roundPrecision)
                        pm_token = round(equity / 8, roundPrecision)
                        debt_token_num = round(1, roundPrecision)
                        real_content.extend([str(equity), str(debt_token_num), str(equity - debt_token_num), str(vl_token), str(m_token), str(pm_token)])
            
        else:
            target_tokens_count = total_tokens
            debt_value = 0
            for p in range(target_tokens_count):
                token_name = token_names[p]
                roundPrecision = 8
                if token_name in SpecialToken:
                    roundPrecision = 2
                equity = round(random.uniform(0, 1000), roundPrecision)
                vl_token = round(equity / 2, roundPrecision)
                m_token = round(equity / 4, roundPrecision)
                pm_token = round(equity / 8, roundPrecision)
                debt_value += get_debt_value(token_name, vl_token, m_token, pm_token, token2vltiersratio, token2margintiersratio, token2pmtiersratio, token2price)
                real_content.extend([str(equity), str(0), str(0), str(vl_token), str(m_token), str(pm_token)])

            average_debt_value = debt_value // target_tokens_count
            for p in range(target_tokens_count):
                debt_token = get_debt_token_by_value(token_names[p], average_debt_value, token2price)
                roundPrecision = 8
                if token_names[p] in SpecialToken:
                    roundPrecision = 2
                debt_token = round(debt_token * 0.99, roundPrecision)
                real_content[3 + 6 * p] = str(debt_token)

            for p in range(total_tokens-target_tokens_count):
                real_content.extend(["0.0", "0.0", "0.0", "0.0", "0.0", "0.0"])
        
        real_content.append("0.0")
        content.append(real_content)

    file_name = "sample_users" + str(id) + ".csv"

    with open(file_name, "w") as csv_file:
        w = csv.writer(csv_file, delimiter=',', quotechar='"', quoting=csv.QUOTE_MINIMAL)
        w.writerows(content)
        print("finish handling ", file_name)

def get_debt_token_by_value(token_name, debt_value, token2price):
    debt_token_num = 0
    roundPrecision = 8
    if token_name in SpecialToken:
        debt_token_num = debt_value / token2price[token_name] / SpecialTokenNumMultiplier
        roundPrecision = 2
    else:
        debt_token_num = debt_value / token2price[token_name] / NormalTokenNumMultiplier
    
    debt_token_num = round(debt_token_num, roundPrecision)
    return debt_token_num

def get_debt_value(token_name, vl_token_num, m_token_num, pm_token_num,
                vltoken2tiersratio, mtoken2tiersratio, pmtoken2tiersratio, token2price):
    vl_token_value = get_collateral_value(token_name, vl_token_num, vltoken2tiersratio, token2price)
    m_token_value = get_collateral_value(token_name, m_token_num, mtoken2tiersratio, token2price)
    pm_token_value = get_collateral_value(token_name, pm_token_num, pmtoken2tiersratio, token2price)
    debt_token_value = vl_token_value + m_token_value + pm_token_value
    return debt_token_value

def get_debt_token_num(token_name, vl_token_num, m_token_num, pm_token_num,
                vltoken2tiersratio, mtoken2tiersratio, pmtoken2tiersratio, token2price):
    debt_token_value = 10 + get_debt_value(token_name, vl_token_num, m_token_num, pm_token_num, vltoken2tiersratio, mtoken2tiersratio, pmtoken2tiersratio, token2price)
    debt_token_value = debt_token_value / token2price[token_name]
    debt_token_num = 0
    roundPrecision = 8
    if token_name in SpecialToken:
        debt_token_num = debt_token_value / SpecialTokenNumMultiplier
        roundPrecision = 2
    else:
        debt_token_num = debt_token_value / NormalTokenNumMultiplier
    
    debt_token_num = round(debt_token_num, roundPrecision)
    return debt_token_num


def get_collateral_value(token_name, token_num, token2tiersratio, token2price):
    if token_name in SpecialToken:
        token_num = token_num * SpecialTokenNumMultiplier
    else:
        token_num = token_num * NormalTokenNumMultiplier

    token_value = token_num * token2price[token_name]

    tiers = token2tiersratio[token_name]
    real_value = 0
    index = 0
    last_boundary = 0
    for item in tiers:
        if token_value <= item[0]:
            break
        real_value += (item[0] - last_boundary) * item[1] // 100
        index += 1
        last_boundary = item[0]
    
    if index < len(tiers):
        real_value += (token_value - last_boundary) * tiers[index][1] // 100
    return real_value


if __name__ == "__main__":
    if len(sys.argv) != 4:
        print("should specify the id, total account counts, and invalid account counts")
        exit(1)
    id = int(sys.argv[1])
    account_counts = int(sys.argv[2])
    invalid_account_counts = int(sys.argv[3])
    generate_data(id, account_counts, invalid_account_counts)
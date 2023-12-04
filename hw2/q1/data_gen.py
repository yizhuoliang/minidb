import random
import sys
import pandas as pd

def generate_data(fract, n):
    p = list(range(1, n + 1))
    random.shuffle(p)

    outv = p
    while len(p) > 1:
        p = p[:int(len(p) * fract)]
        outv = p + outv

    return outv

def populate_csv(symbols, total_rows):
    trades = []
    prices = {symbol: random.randint(50, 500) for symbol in set(symbols)}
    time = 1

    for _ in range(total_rows):
        symbol = random.choice(symbols)
        quantity = random.randint(100, 10000)
        last_price = prices[symbol]
        price = random.randint(max(50, last_price - 5), min(500, last_price + 5))
        prices[symbol] = price
        trades.append(("s" + str(symbol), time, quantity, price))
        time += 1

    df = pd.DataFrame(trades, columns=['stocksymbol', 'time', 'quantity', 'price'])
    df.to_csv('trades.csv', index=False)

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Insufficient arguments!")
        sys.exit(1)
    
    fract = float(sys.argv[1])
    n = int(sys.argv[2])
    p = generate_data(fract, n)
    populate_csv(p, 10000000)
    
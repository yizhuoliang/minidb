import csv
import random

x = 15  # number of lines per id
max_id = 100000  # maximum id
filename = 'test_data1.csv'  # name of the output file

def generate_random_value():
    return random.randint(1000, 99999)

rows = []

for id in range(1, max_id + 1):
    for _ in range(x):
        rows.append([id, generate_random_value()])

# shuffle
random.shuffle(rows)

with open(filename, mode='w', newline='') as file:
    writer = csv.writer(file)
    writer.writerow(['id', 'value'])  # Writing header
    writer.writerows(rows)  # Writing shuffled rows

print(f'CSV file "{filename}" generated with {max_id} ids and {x} lines per id, shuffled.')

FROM node:22-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY index.html ./
COPY src ./src
RUN npm run build

FROM node:22-alpine

RUN npm install -g serve@14.2.4 --no-audit --no-fund

WORKDIR /app
COPY --from=build /app/dist ./dist

EXPOSE 5173
CMD ["serve", "-s", "/app/dist", "-l", "5173", "--no-clipboard", "--no-port-switching"]

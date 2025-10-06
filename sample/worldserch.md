# **Resoniteワールド検索プログラム実装計画**

## **1\. 概要**

### **目的**

ユーザーが指定したキーワードでResoniteの公開ワールドを検索し、選択したワールドのレコードURL (resonite:///...) を取得するプログラムを実装する。

### **手法**

公式サイト go.resonite.com の検索結果ページから直接情報を取得する**HTMLスクレイピング**方式を採用する。

## **2\. 実装手順**

### **ステップ1：HTTPリクエスト送信**

プログラムは、指定されたキーワードをパラメータとして go.resonite.com にGETリクエストを送信し、検索結果ページのHTMLを取得します。

* **リクエスト先URL**: https://go.resonite.com/world  
* **パラメータ**: term={検索キーワード}  
* **リクエストURLの例**:  
  \[https://go.resonite.com/world?term=Metaworld\](https://go.resonite.com/world?term=Metaworld)

### **ステップ2：HTMLレスポンスの解析**

サーバーから返されたHTMLドキュメントの中から、ワールド一覧が記載されているリスト部分を特定し、個々のワールド情報を解析します。

* **解析対象**: olタグ（クラス名: listing）内の各liタグ。  
* **HTML構造の例**:  
  \<ol class="listing"\>  
      \<li\> \<\!-- 1つ目のワールド情報 \--\>  
          \<a class="listing-item" href="/world/U-ユーザーID/R-レコードID"\>  
              \<h2 class="listing-item\_\_heading"\>  
                  \<span\>ワールド名\</span\>  
              \</h2\>  
              ...  
          \</a\>  
      \</li\>  
      ...  
  \</ol\>

### **ステップ3：情報抽出**

解析対象のHTML構造から、以下の2つの情報をワールドごとに抽出します。

1. **ワールド名**:  
   * **抽出元**: \<h2\> タグ（クラス名: listing-item\_\_heading）のテキスト。  
   * **用途**: ユーザーに提示する選択肢リストの作成。  
2. **ワールドの相対URL**:  
   * **抽出元**: \<a\> タグ（クラス名: listing-item）の href 属性値。  
   * **用途**: レコードIDを特定するための元データ。  
   * **形式**: /world/U-(ユーザーID)/R-(レコードID)

### **ステップ4：レコードURLの生成**

抽出した相対URLから、Resoniteクライアントが認識できる形式の最終的なURLを組み立てます。

* **レコードIDの特定**: 相対URLから R- で始まる部分文字列を正規表現などで抽出します。  
  * **例**: /world/.../R-xxxxxxxx → R-xxxxxxxx  
* **最終URLの生成**: resonite:///world/ というプレフィックスに、抽出したレコードIDを結合します。  
  * **例**: resonite:///world/R-xxxxxxxx

## **3\. 処理フロー図**

graph TD  
    A\[開始\] \--\> B{ユーザーが\<br\>キーワードを入力};  
    B \--\> C\[リクエストURLを生成\<br\>\<small\>https://.../?term=KEYWORD\</small\>\];  
    C \--\> D\[HTMLを取得\];  
    D \--\> E\[HTMLから\<br\>ワールド名と相対URLを抽出\];  
    E \--\> F{ワールド名の一覧を表示};  
    F \--\> G{ユーザーが\<br\>ワールドを選択};  
    G \--\> H\[相対URLから\<br\>レコードIDを特定\];  
    H \--\> I\[最終的なレコードURLを生成\<br\>\<small\>resonite:///...\</small\>\];  
    I \--\> J{結果を出力};  
    J \--\> K\[終了\];

## **4\. 推奨ツール・ライブラリ**

* **プログラミング言語**: Python  
* **ライブラリ**:  
  * requests: HTTPリクエストの送信に利用。  
  * BeautifulSoup4: HTMLの解析と情報抽出に利用。